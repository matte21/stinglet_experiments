#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../../shared/env.sh"

if [[ "$#" -ne 5 ]]; then
    echo "Fatal: invalid arguments. Usage:"
    echo "$0 <comma separated list of azure VMs IDs> <comma separated list of CXL VM percentages names, or empty string> <results directory> <kubelet bin> <kubelet cfg>"
    exit 1
fi
readonly machine_ids="$1"
readonly cxl_percentage_db_column="${2:-nop}"
readonly results_dir="$3"
readonly kubelet_bin="$4"
readonly kubelet_cfg="$5"

# Make sure all tables for the traces we have to play do exist.
for mid in ${machine_ids//,/ }; do
    if ! sqlite3 "$trace_db_file" ".tables" | grep -qw "events-machine-id-${mid}"; then
        echo "Fatal: table events-machine-id-${mid} for machine ID $mid requested in experiment doesn't exist."
        exit 1
    fi
done

for mid in ${machine_ids//,/ }; do
    for cpdc in ${cxl_percentage_db_column//,/ }; do
        kubelet_type="standard"
        if [[ "$kubelet_bin" == "stinglet" ]]; then
            kubelet_type="stinglet"
        fi
        log_file="$results_dir/${kubelet_bin}_${kubelet_cfg}.log"

        # Launch the binary that plays the trace. It sucks to use a separate binary
        # (and written in a language other than bash!) rather than playing the trace directly here.
        # But playing the traces directly in bash is too slow. i.e. just iterating over all trace
        # events (all arrivals and departures), without processing them, took ~12 minutes per-trace.
        # Considering all traces and baselines we want to play, that would have been a 30 hours
        # overhead to play all traces. With this bibnary (written in Go), the per-trace overhead is
        # is only 30 secs, and the cumulative overhead across all traces is ~2 hours.
        sudo helpers/play_trace --trace-db "$trace_db_file" \
            --trace-machine-id "$mid" \
            --cxl-percentage-db-column "$cpdc" \
            --results-dir "$results_dir" \
            --results-file-prefix "${kubelet_bin}_${kubelet_cfg}_${mid}" \
            --kubelet-type "$kubelet_type" \
            --timeout "10m" > "$log_file" 2>&1

        # play_trace cleans up the pods but exits as soon as kubelet cleared their state. But the
        # containers state might still exist, because the container runtime cleans them up
        # asynchronously. Before we play the next trace, makes sure that the container runtime
        # deleted everything.
        while [[ $(sudo crictl pods -q) != "" || $(sudo crictl ps -q) != "" ]]; do
            echo "Waiting for all pods to have been permanently deleted..."
            sleep 5
        done
        echo All pods and containers have been permanently deleted.
    done
done
