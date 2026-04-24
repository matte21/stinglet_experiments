#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

if [[ "$#" -lt 5 ]]; then
    echo "Fatal: invalid arguments."
    echo "Usage: $0 <mem sampling enabled? on/off> <experiment name> <pod schedule name> <path to the folder where to store collected data> <name of first pod to create> [ <name of 2nd pod to create> ... <name of Nth pod to create> ]"
    exit 1
fi
readonly mem_sampling_on="$1"
readonly experiment_name="$2"
readonly pod_schedule_name="$3"
readonly root_output_folder="$4"
shift 4

# Create pods one at a time in the order that they were passed.
for pod in "$@"; do
    echo creating pod $pod.yaml
    # We add a random label to the pod manifest because the kubelet generates the UID by
    # hashing the pod manifest. Without this trick, the same pod manifest gets the same UID
    # across different experiments and traces. That's bad, because the kubelet cleans up some of the
    # pods state lazily, and identifies pods by UID; the kubelet could mistakenly assume that a
    # newly created pod has already completed because it still finds the state corresponding to
    # that pod in a previous experiment.
    yq eval ".metadata.labels.unique = \"$RANDOM\"" sys-b-pods/$pod.yaml | sudo tee /etc/kubernetes/manifests/$pod.yaml > /dev/null
    sleep 1
done

readonly output_folder="$root_output_folder/$experiment_name/$pod_schedule_name"
mkdir -p "$output_folder"

readonly cdf_file="$output_folder/cdf.csv"
echo "$experiment_name,,,,,," > $cdf_file
echo "seconds since t0,num apps completed,,,," >> $cdf_file

if [[ "$mem_sampling_on" == "on" ]]; then
    ./start_mem_sampling.sh "$output_folder" &
    mem_sampler_pid=$!
    trap "kill $mem_sampler_pid || true" EXIT
fi

hostname=$(hostname)

# Periodically poll the status of each pod, until they are all done.
# For now, we just wait for all pods to be in succeeded/failed state since they're all batch jobs.
# At some point we'll have never-terminating pods representing long-running services, so we'll
# need to do things differently. We also take the chance to record how long each container/pod
# took to complete, since we need to measure application performance, and the memory and cpu cpusets
# of each pod.
readonly poll_period=3
num_polls=1
readonly kubelet_pods_api="http://localhost:10255/pods"
readonly num_pods=$#
num_succeeded=0
pending_pods=()
declare -A unadmitted_pods=()
# "other reasons" as in "other reasons than admission errors"
declare -A pods_failed_for_other_reasons=()
declare -A cpus_assignments=()
declare -A mem_nodes_assignments=()
declare -A max_per_node_assignments=()
# We record the runtime of each pod at second resolution. We assume that all pods take at least 1
# minute to complete, so seconds are fine-grained enough. All pods that we currently use meet this
# assumption.
declare -A runtime_secs
while (( num_succeeded + ${#unadmitted_pods[@]} + ${#pods_failed_for_other_reasons[@]} < num_pods )); do
    pending_pods=()
    unadmitted_pods=()
    pods_failed_for_other_reasons=()

    num_succeeded=0
    num_running=0
    num_unknown=0

    # For each pod extract name, phase (so that we can easily tell whether it succeeded), ID of its
    # only container, start time and end time (so that we can compute its runtime in case it succeeded).
    # We use containerStatuses[0] because all of the pods we use have exactly one container. If that
    # changes, the following jq expression no longer works.
    pods_statuses=($(curl -s "$kubelet_pods_api" | jq -r '.items[] | "\(.metadata.name) \(.status.phase) \(.status.containerStatuses[0].containerID) \(.status.containerStatuses[0].state.terminated.startedAt) \(.status.containerStatuses[0].state.terminated.finishedAt) \(.status.containerStatuses[0].state.terminated.reason) \(.status.reason)"'))

    while [[ "${#pods_statuses[@]}" -gt 0 ]]; do
        p_name=${pods_statuses[0]}
        p_phase=${pods_statuses[1]}

        if [[ "$p_phase" == "Succeeded" ]]; then
            num_succeeded="$((num_succeeded+1))"
            start_time_secs=$(date -u -d "${pods_statuses[3]}" +%s)
            end_time_secs=$(date -u -d "${pods_statuses[4]}" +%s)
            runtime_secs[$p_name]=$((end_time_secs - start_time_secs))
        elif [[ "$p_phase" == "Pending" ]]; then
            pending_pods+=("$p_name")
        elif [[ "$p_phase" == "Failed" ]]; then
            # Unfortunately the kubelet API exposes the failure reason in two different fields. Sometimes one is set,
            # sometimes, the other. So we have to check both.
            failure_reason_1=${pods_statuses[5]}
            failure_reason_2=${pods_statuses[6]}
            if [[ "$failure_reason_1" == "UnexpectedAdmissionError" || "$failure_reason_2" == "UnexpectedAdmissionError" ]]; then
                unadmitted_pods[$p_name]=""
            else
                pods_failed_for_other_reasons[$p_name]=${failure_reason_1:-$failure_reason_2}
            fi
        elif [[ "$p_phase" == "Running" ]]; then
            num_running="$((num_running+1))"
            
            # the format of ${pods_statuses[2]} is "<type>://<container_id>", we only want "<container_id>"", hence we cut.
            container_id=$(echo "${pods_statuses[2]}" | cut -d '/' -f 3)

            # Record which CPUS and NUMA nodes (for memory) the kubelet assigned to this pod via
            # cgroups.
            if [[ ! -v "cpus_assignments[$p_name]" ]]; then
                cgroup=$(crictl inspect "$container_id" | jq '.info.runtimeSpec.linux.cgroupsPath' | tr -d \")
                cgroup_dir="/sys/fs/cgroup$cgroup"
                cpus_file="$cgroup_dir/cpuset.cpus"
                mem_nodes_file="$cgroup_dir/cpuset.mems"
                max_per_node_file="$cgroup_dir/memory.max_per_node"
                if [[ ! -z "$cgroup" && -d "$cgroup_dir" && -f "$cpus_file" && -f "$mem_nodes_file" && -f "$max_per_node_file" ]]; then
                    # We translate ',' to ';' because we'll store the value in a csv file, and we want it to be
                    # the value of a single cell, rather than be (spuriously) split across multiple cells (by default csv
                    # files use ',' as the cell separator). We also translate '-' to 'R' because sometimes the cgroups files
                    # contain ranges of values in the format 'start-end', but excel interprets cells with such a value as
                    # dates where 'start' is the day of the month and 'end' is the month, and of course we don't want that.
                    cpus_assignments[$p_name]=$(cat "$cpus_file" | tr ',' ';' | tr '-' 'R')
                    mem_nodes_assignments[$p_name]=$(cat "$mem_nodes_file" | tr ',' ';' | tr '-' 'R')
                    max_per_node_assignments[$p_name]=$(cat "$max_per_node_file" | tr '\n' ';' | tr -s ' ' | tr ' ' '=')
                    echo "$p_name max per node assignments ${max_per_node_assignments[$p_name]}"
                else
                    echo "cgroup of pod $p_name does not exist yet..."
                fi
            fi

            # Start a background process that samples the per-NUMA node memory usage of the
            # pod's container. We also set a trap so that it is always killed when this script exits (e.g. due
            # to errors). Knowing the per-NUMA Node memory usage helps us debug performance
            # issues and weird experimental results (e.g. pods being super slow because all of their
            # memory is on CXL).
            # mem_usage_file="$output_folder/${p_name/\-$hostname/}.mem_usage_gibs.csv"
            # if [[ ! -f "$mem_usage_file" ]]; then
            #     num_numa_nodes=$(lscpu | grep "NUMA node(s)" | tr -s ' ' | cut -d ' ' -f 3)
            #     # Add a heading line with the IDs of all the NUMA nodes.
            #     echo ",$(seq 0 $((num_numa_nodes - 1)) | paste -sd,)," > "$mem_usage_file"
            #     # Note: sample_memory_usage.sh kills itself when the container it's sampling terminates,
            #     # so we don't need to explicitly kill it (except for abrupt/unexpected terminations
            #     # of this script, which is why we set the trap).
            #     ./sample_memory_usage.sh $container_id $mem_usage_file $num_numa_nodes &
            #     trap "kill $! 2>/dev/null || true" EXIT ERR
            # fi
        elif [[ "$p_phase" == "Unknown" ]]; then
            num_unknown="$((num_unknown+1))"
        else
            echo "kubelet returned pod phase \"$p_phase\" which should not exist, only existing phases are Succeeded, Failed, Running, Pending and Unknown."
            exit 1
        fi

        pods_statuses=("${pods_statuses[@]:7}")
    done

    echo
    echo "Live pods stats:"
    echo "Succeeded: $num_succeeded / $num_pods"
    echo "Failed at admission:    ${#unadmitted_pods[@]} / $num_pods; pods: ${unadmitted_pods[@]}"
    pods_and_reasons=""
    for pod in ${!pods_failed_for_other_reasons[@]}; do
        pods_and_reasons+="($pod, ${pods_failed_for_other_reasons[$pod]});"
    done
    echo "Failed for other reasons:    ${#pods_failed_for_other_reasons[@]} / $num_pods; pods: $pods_and_reasons"
    echo "Running:   $num_running / $num_pods"
    echo "Pending:   ${#pending_pods[@]} / $num_pods; pods: ${pending_pods[@]}"
    echo "Unknown:   $num_unknown / $num_pods"

    echo "$((poll_period * num_polls)),$num_succeeded,,,,," >> $cdf_file
    num_polls=$((num_polls + 1))

    sleep $poll_period
done

if [[ "$mem_sampling_on" == "on" ]]; then
    kill "$mem_sampler_pid" || true
fi

# Get the logs of each pod. We use them to manually double-check that everything ran correctly,
# i.e. to ensure that the experiment run was clean and we can trust the experiment results.
# We could do this in the other loop over all the pods further down below, where we write all
# the csv files with the results. But experience taught me that for debugging bugs that cause
# the script to exit before that loop is reached, having the pods logs is very useful, hence we
# fetch them as soon as possible.
for pod in "$@"; do
    # Only pods that have been admitted have logs.
    if [[ -z "${unadmitted_pods[${pod}-${hostname}]+x}" ]]; then
        # Note that we grep on "$pod-", with a trailing "-". Without it, it can happen that when
        # we grep for a pod called foo-1, there's a pod called foo-2 with a container called "foo"
        # whose complete log file name has form foo-2-<other-stuff>-foo-<hash> where hash begins
        # with "1"; the second "foo" in the file name is the container name. In those cases,
        # grep would match foo-2 log file as well (besides the correct file for foo-1). Appending
        # "-" saves us from that.
        log_file=$(ls /var/log/containers | grep "$pod-")
        cp /var/log/containers/$log_file $output_folder/$pod.logs
    fi
done

echo

if [[ "${#pods_failed_for_other_reasons[@]}" -gt 0 ]]; then
    echo "Some pods failed due to errors other than admission errors. It should never happen, aborting experiment now."
    echo "You can see which pods failed and why in the messages above this one."
    exit 1
fi

# After a pod succeeds, its cgroups are deleted. This script is susceptible to race conditions where
# a pod succeeds before the contents of its cgroups files are read, which means we won't be able
# to read those files period. Practically that's unlikely, since every pod we currently use runs
# for at least one minute and we check its cgroups every few seconds. But better safe than sorry.
# Also note that it's ok for pods to fail due to admission errors, but if that happens, the pod
# never gets to run and doesn't have cpus and memory assigned via cgroups. So we must
# account for the fact that we didn't record the cpu assignments for pods failed with admission
# errors.
if (( ${#cpus_assignments[@]} + ${#unadmitted_pods[@]} < $num_pods )); then
    echo "Couldn't read the cgroups files for some of the successfully completed pods, so the results are incomplete."
    echo "This should never happen."
    echo "Follow the cgroups that could be read, for debugging:"
    for pod in ${!cpus_assignments[@]}; do
        echo "$pod: ${cpus_assignments[$pod]}" 
    done
    # todo: print some guidance on how to clean up.
    echo "Aborting experiment, please clean up and re-run experiment."
    exit 1
fi

if [[ ${#unadmitted_pods[@]} -eq 0 ]]; then
    echo "All pods have completed successfully."
else
    echo "Some pods failed admission, see previous live pods stats message to find out which ones."
fi

readonly cpus_assignments_file="$output_folder/cpus.csv"
readonly mem_nodes_assignments_file="$output_folder/mem_nodes.csv"
readonly max_per_node_assignments_file="$output_folder/max_per_node.csv"
readonly runtimes_file="$output_folder/runtimes_seconds.csv"

# Collect the data to write to files.
heading_line="$pod_schedule_name,a value of X means the pod has been rejected at admission time,"
pods_line=",,"
cpus_line=",$experiment_name,"
mem_nodes_line=",$experiment_name,"
max_per_node_line=",$experiment_name,"
runtimes_line=",$experiment_name,"
for pod in "$@"; do
    if [[ "$pod" == silo* ]]; then
        heading_line+=",,"
        pods_line+="$pod-lat (ms),$pod-tput (ops / sec),"
    else
        heading_line+=","
        pods_line+="$pod,"
    fi

    # Pods that were unadmitted have no recorded runtimes, cpus, and mem nodes assignments. So for
    # them we store "-1" as a placeholder in their csv files.
    if [[ -v cpus_assignments["$pod-$hostname"] ]]; then
        # The value of variable "pod" is the pod name as written in the .metadata.name field of the
        # pod's YAML file. But the actual pod name as seen by k8s is that name followed by -hostname
        # (the kubelet appends the hostname). The following arrays are keyed by the actual pod name,
        # hence we have to append the hostname to $pod when we do lookups.    
        cpus_line+="${cpus_assignments[$pod-$hostname]},"
        mem_nodes_line+="${mem_nodes_assignments[$pod-$hostname]},"
        max_per_node_line+="${max_per_node_assignments[$pod-$hostname]},"

        if [[ "$pod" == silo* ]]; then
            cpus_line+="${cpus_assignments[$pod-$hostname]},"
            mem_nodes_line+="${mem_nodes_assignments[$pod-$hostname]},"
            max_per_node_line+="${max_per_node_assignments[$pod-$hostname]},"
            lat=$(grep "avg_latency" "$output_folder/$pod.logs" | cut -d ' ' -f 5-)
            # sometimes, the actual latency value gets printed on a separate line!
            if [[ "$(echo $lat | tr -s ' ')" == "" ]]; then
                lat=$(grep -A 1 "avg_latency" "$output_folder/$pod.logs" | tail -1 | cut -d ' ' -f 4-)
            fi
            lat="$(echo $lat | sed 's/ ms//')"
            tput=$(grep "agg_throughput" "$output_folder/$pod.logs" | cut -d ' ' -f 5)
            runtimes_line+="$lat,$tput,"
        else
            runtimes_line+="${runtime_secs[$pod-$hostname]},"
        fi
    else
        cpus_line+="X,"
        mem_nodes_line+="X,"
        max_per_node_line+="X,"
        runtimes_line+="X,"
        if [[ "$pod" == silo* ]]; then
            cpus_line+="X,"
            mem_nodes_line+="X,"
            max_per_node_line+="X,"
            runtimes_line+="X,"
        fi
    fi
done

echo "$heading_line" > $cpus_assignments_file
echo "$heading_line" > $mem_nodes_assignments_file
echo "$heading_line" >> $max_per_node_assignments_file
echo "$heading_line" > $runtimes_file

echo "$pods_line" >> $cpus_assignments_file
echo "$pods_line" >> $mem_nodes_assignments_file
echo "$pods_line" >> $max_per_node_assignments_file
echo "$pods_line" >> $runtimes_file

echo "$cpus_line" >> $cpus_assignments_file
echo "$mem_nodes_line" >> $mem_nodes_assignments_file
echo "$max_per_node_line" >> $max_per_node_assignments_file
echo "$runtimes_line" >> $runtimes_file

# Delete all pods and wait for them to have been completely deleted.
sudo rm -rf /etc/kubernetes/manifests/*
while [[ $(curl -s "$kubelet_pods_api" | jq '.items') != "null" || $(crictl pods -q) != "" || $(crictl ps -q) != "" ]]; do
    echo "Waiting for all pods to have been permanently deleted..."
    sleep 5
done
echo All pods and containers have been permanently deleted.
