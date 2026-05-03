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
readonly cxl_vms_percentages_names="${2:-nop}"
readonly results_dir="$3"
readonly kubelet_bin="$4"
readonly kubelet_cfg="$5"

# Make sure all tables for the traces we have to play do exist.
for mid in ${machine_ids//,/ }; do
    if ! sqlite3 "../$trace_db_file" ".tables" | grep -qw "events-machine-id-${mid}"; then
        echo "Fatal: table events-machine-id-${mid} for machine ID $mid requested in experiment doesn't exist."
        exit 1
    fi
done

for mid in ${machine_ids//,/ }; do
    for cvpn in ${cxl_vms_percentages_names//,/ }; do
        # Launch the binary that plays the trace. It sucks to use a separate binary
        # (and written in a language other than bash!) rather than playing the trace directly here.
        # But playing the traces directly in bash is too slow. i.e. just iterating over all trace
        # events (all arrivals and departures), without processing them, took ~12 minutes per-trace.
        # Considering all traces and baselines we want to play, that would have been a 30 hours
        # overhead to play all traces. With this bibnary (written in Go), the per-trace overhead is
        # is only 30 secs, and the cumulative overhead across all traces is ~2 hours.
    done
done