#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../../shared/env.sh"

if [[ "$#" -ne 3 ]]; then
    echo "Fatal: invalid arguments. Usage:"
    echo "$0 <comma separated list of azure VMs IDs> <comma separated list of CXL VM percentages names, or empty string> <results directory>"
    exit 1
fi
readonly machine_ids="$1"
readonly cxl_vms_percentages_names="${2:-nop}"
readonly results_dir="$3"

# Make sure all tables for the traces we have to play do exist.
for mid in ${machine_ids//,/ }; do
    if ! sqlite3 "../$trace_db_file" ".tables" | grep -qw "events-machine-id-${mid}"; then
        echo "Fatal: table events-machine-id-${mid} for machine ID $mid requested in experiment doesn't exist."
        exit 1
    fi
done

for mid in ${machine_ids//,/ }; do
    for cvpn in ${cxl_vms_percentages_names//,/ }; do
        # start collection of kubelet admission times for all containers.
        # todo: need to set output folder.
        journalctl -f -u kubelet | >> $results_dir/.csv &
        echo $! > admission_time_observer_pid

        # 
    done
done