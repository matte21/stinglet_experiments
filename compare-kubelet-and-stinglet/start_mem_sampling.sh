#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

if [[ "$#" -ne 1 || "$1" == "" ]]; then
    echo "Fatal: invalid arguments." >&2
    echo "Usage: $0 <output directory>" >&2
    exit 1
fi
readonly out_dir="$1"
if [[ ! -d "$out_dir" ]]; then
    echo "Fatal: $0 invalid first argument (output directory), directory \"$out_dir\" doesn't exist." >&2
    exit 1
fi

readonly log_file="$out_dir/mem_sampler.log"
readonly samples_file_suffix="mibs_per_node_samples.csv"

echo "Mem sampler started" > $log_file

while true; do
    echo "Sampling per-process, per NUMA node memory consumption" >> $log_file

    # cid stands for "container id". In general, "c" is a prefix that stands for "container".
    while IFS= read -r cid; do
        [[ -z "$cid" ]] && continue

        cinfo="$(sudo crictl inspect $cid || true)"
        if [[ "$cinfo" == "" ]]; then
            # The container has terminated in the meantime.
            continue
        fi

        name="$(echo $cinfo | jq -r '.status.metadata.name')"

        main_pid=$(echo "$cinfo" | jq -r '.info.pid')
        proc_pid_1=""
        proc_pid_2=""

        # For some containers the process where the workload runs is not the main process in the
        # container, but a child of it, hence we retrieve the PID of that child process.
        case "$name" in
            *xsbench*)
                proc_pid_1=$(sudo pgrep --ns $main_pid -f "^./xsbench" || true)
                ;;
            *bwaves-s*)
                proc_pid_1=$(sudo pgrep --ns $main_pid -f "^./speed_bwaves_base.mytest-m64" || true)
                ;;
            *roms-s*)
                proc_pid_1=$(sudo pgrep --ns $main_pid -f "^./sroms_base.mytest-m64" || true)
                ;;
            *graph500-xl*)
                # graph500-xl runs two separate processes inside it, so we need two PIDs. 
                proc_pid_1=$(sudo pgrep --ns $main_pid -f "^/omp-csr -s 27 -e 15 -V" || true)
                proc_pid_2=$(sudo pgrep --ns $main_pid -f "^/omp-csr -s 27 -e 14 -V" || true)
                ;;
            *)
                proc_pid_1=$main_pid
                ;;
        esac

        for pid_to_sample in $proc_pid_1 $proc_pid_2; do
            # Apologies for the sed madness (but I like sed a lot).
            samples=$(sudo numastat -p $pid_to_sample | \
                grep Total | \
                tr -s ' ' | \
                sed 's/\([0-9]\) /\1,/g' | \
                sed 's/Total//' | \
                sed 's/Node //g' | \
                sed 's/^ //' | \
                sed 's/,[^,]*$//')

            if [[ "$samples" == "" ]]; then
                # The process has terminated.
                continue
            fi

            pod_name=$(echo "$cinfo" | jq -r '.info.runtimeSpec.annotations["io.kubernetes.pod.name"]')

            if [[ "$pid_to_sample" == "$proc_pid_1" ]]; then
                samples_file="${out_dir}/1-${pod_name}_${samples_file_suffix}"
            else
                samples_file="${out_dir}/2-${pod_name}_${samples_file_suffix}"
            fi

            if [[ -f "$samples_file" ]]; then
                # Only print the second line with the readings, avoid the heading line with the nodes IDs.
                # Note that even the output is originally split across two lines, bash merges them
                # in a single line with a blank space separator, hence we cut based on ' ', not '\n'. 
                echo $samples | cut -d ' ' -f 2 >> "$samples_file"
            else
                # This is the first time we write to the file, so we include both the samples and the
                # heading line with the node IDs.
                echo $samples | tr ' ' '\n' >> "$samples_file"
            fi
        done
    done < <(sudo crictl ps -q)

    sleep 3
done
