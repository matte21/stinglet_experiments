#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

source ../shared/env.sh

# Check invocation arguments.
if [[ -z "${1:-}" ]]; then
    echo "Fatal: invalid arguments."
    echo "Usage: $0 <file name of experiment description file in dir \"./experiments_descriptions\">"
    exit 1
fi
readonly experiment_file="$1"

# Start the container runtime, CRI-O.
sudo systemctl start crio.service
while [[ "$(sudo systemctl is-active crio.service)" != "active" ]]; do
    sleep 1
    echo waiting for cri-o service to get up and running.
done

# If we don't have the file holding the Azure arrival traces, create it.
if [[ ! -f "$trace_db_file" ]]; then
    ./helpers/prepare_azure_trace.sh
else
    echo "Azure trace already present, not downloading it."
fi

# Prepare folder where we'll store experiments data.
readonly results_dir="collected-data/$(date +'%Y-%m-%d-%H-%M-%S')"
mkdir -p "$results_dir"
cp "experiments-descriptions/$experiment_file" "$results_dir"

# Store the whole experiments description file in an array, where each item holds a single line.
mapfile -t experiments_desc < <(grep . experiments-descriptions/$experiment_file)

# Parse the experiments description file line by line. Once we parse the last line that makes up
# an experiment, run that experiment. Then, proceed to parse the lines that describe the next
# experiment, and run that experiment, etc... .
while [[ "${#experiments_desc[@]}" -gt 0 ]]; do

    # The experiment with name "experiment_I" is described by a contiguous group of lines whose
    # first line is "[experiment_I]". So when we encounter one such line, we know we're beginning
    # a new experiment, and we need to parse the lines that describe that particular experiment.
    # It's always the first line in the experiment description.
    experiment_name=$(echo "${experiments_desc[0]}" | tr -d '[]')

    # A given experiment always uses a particular kubelet binary (which might be the standard
    # kubelet or a custom kubelet modified by me). It's always the second line in the experiment
    # description.
    kubelet_bin=$(echo "${experiments_desc[1]}" | cut -d '=' -f 2)
    if [[ ! -x "$kubelet_bins_dir/$kubelet_bin" ]]; then
        echo "Fatal: experiment $experiment_name uses kubelet bin $kubelet_bin, but no such executable exists at $kubelet_bins_dir."
        exit 1
    fi

    # A given experiment always uses a particular kubelet configuration file. It's always the third
    # line in the experiment description.
    kubelet_cfg=$(echo "${experiments_desc[2]}" | cut -d '=' -f 2)
    if [[ ! -f "$kubelet_cfgs_dir/$kubelet_cfg" ]]; then
        echo "Fatal: experiment $experiment_name uses kubelet config $kubelet_cfg, but no such file exists at $kubelet_cfgs_dir"
        exit 1
    fi

    azure_machine_ids=$(echo "${experiments_desc[3]}" | cut -d '=' -f 2)
    cxl_vms_percentages_names=$(echo "${experiments_desc[4]}" | cut -d '=' -f 2)
    if [[ "$kubelet_bin" != "stinglet" ]]; then
        # only stinglet understands that some VMs want a fraction of their memory in CXL.
        cxl_vms_percentages_names=""
    fi

    echo "Starting kubelet with binary $kubelet_bin and config file $kubelet_cfg, experiment $experiment_name"
    ./helpers/start_kubelet.sh "$kubelet_bins_dir/$kubelet_bin" "$kubelet_cfgs_dir/$kubelet_cfg"

    ./helpers/play_traces.sh "$azure_machine_ids" "$cxl_vms_percentages_names" "$results_dir" "$kubelet_bin" "$kubelet_cfg"

    echo "Stopping kubelet with binary $kubelet_bin and config file $kubelet_cfg, experiment $experiment_name"
    ./helpers/stop_kubelet.sh

    # Remove lines of current experiment and move cursor to lines describing the next experiment.
    experiments_desc=("${experiments_desc[@]:5}")
    while [[ "${#experiments_desc[@]}" -gt 0 && -z "${experiments_desc[0]}" ]]; do
        experiments_desc=("${experiments_desc[@]:1}")
    done
done
