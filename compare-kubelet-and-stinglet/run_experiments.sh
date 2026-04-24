#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

# todo: collect managers dumps.
# todo: when running an experiment, record the pods yaml, kubelet config and experiment description. Or, record the commit at which it was run (fo reproducibility).

if [[ "$#" -ne 2 ]]; then
    echo "Fatal: invalid arguments."
    echo "Usage: $0 <file name of experiment description file in dir \"experiments_descriptions\"> <mem sampling enabled? on/off>"
    exit 1
fi
readonly experiment_file="$1"
readonly mem_sampling="$2"

# Start crio.
sudo systemctl start crio.service
while [[ "$(sudo systemctl is-active crio.service)" != "active" ]]; do
    sleep 1
    echo waiting for cri-o service to get up and running.
done

readonly output_folder="collected-data/$(date +'%Y-%m-%d-%H-%M-%S')"
mkdir -p "$output_folder"
cp experiments_descriptions/$experiment_file $output_folder

# Store the whole experiments description file in an array, where each item holds a single line.
mapfile -t experiments_descriptions_file < <(grep . experiments_descriptions/$experiment_file)

# Parse the experiments description file line by line. Once we parse the last line that makes up
# an experiment, run that experiment. Then, proceed to parse the lines that describe the next
# experiment, and run that experiment, etc... .
while [[ "${#experiments_descriptions_file[@]}" -gt 0 ]]; do

    # The experiment with name "experiment_I" is described by a contiguous group of lines whose
    # first line is "[experiment_I]". So when we encounter one such line, we know we're beginning
    # a new experiment, and we need to parse the lines that describe that particular experiment.
    experiment_name=$(echo "${experiments_descriptions_file[0]}" | tr -d '[]')

    # A given experiment always uses a particular kubelet binary (which might be the standard
    # kubelet or a custom kubelet modified by me), and a given kubelet configuration file.
    kubelet_bin=$(echo "${experiments_descriptions_file[1]}" | cut -d '=' -f 2)
    kubelet_cfg=$(echo "${experiments_descriptions_file[2]}" | cut -d '=' -f 2)

    # Folder where we'll store the kubelet logs for this experiment.
    # We store the kubelet logs as experiment metadata. They can be used to check that the kubelet
    # ran as expected, so that we can be sure that the experiment results are valid.
    kubelet_log_folder="$output_folder/$experiment_name"

    # Now, start the kubelet using that binary, with that configuration file.
    echo "running kubelet experiment $experiment_name with binary $kubelet_bin and config file $kubelet_cfg"
    ./start_kubelet.sh $kubelet_bin $kubelet_cfg $kubelet_log_folder

    # Given an experiment, we run multiple "sub-experiments", where in each sub-experiment we
    # play a trace of Pod arrivals on the host. Different subexperiments represent different
    # traces. Before changing sub-experiment, we delete all the pods of the previous one and
    # give the kubelet some time to quiesce. Each sub-experiment is described by a single line,
    # so we read one line, run the sub-experiment, read the next line, run the corresponding
    # sub-experiment, etc... . 
    experiments_descriptions_file=("${experiments_descriptions_file[@]:3}")
    while [[ ! "${experiments_descriptions_file[0]}" =~ "[experiment_" ]]; do

        pods_schedule_name=$(echo "${experiments_descriptions_file[0]}" | cut -d '=' -f 1)
        pods_schedule=( $(echo "${experiments_descriptions_file[0]}" | cut -d '=' -f 2 | tr -s ',' ' ') )

        date

        ./run_pods.sh "$mem_sampling" "$experiment_name" "$pods_schedule_name" "$output_folder" "${pods_schedule[@]}"

        date

        experiments_descriptions_file=("${experiments_descriptions_file[@]:1}")
        [ "${#experiments_descriptions_file[@]}" -gt 0 ] || break
    done

    ./stop_kubelet.sh $kubelet_log_folder
done

sudo systemctl stop crio.service
while [[ "$(sudo systemctl is-active crio.service)" != "inactive" ]]; do
    sleep 1
    echo waiting for crio service to stop.
done
echo Stopped cri-o.

echo Done running all experiments.
