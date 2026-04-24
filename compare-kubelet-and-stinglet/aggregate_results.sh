#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

if [[ "$#" -ne 1 ]]; then
    echo "Invalid arguments"
    echo "Usage: $0 <path to folder with baseline results>"
    exit 1
fi
readonly results_dir="$1"

if [[ ! -d "$results_dir" ]]; then
    echo "Fatal: folder $results_dir doesn't exist."
    exit 1
fi

readonly out_file="$(basename $results_dir).csv"

num_traces=0
for experiment_dir in $results_dir/*; do    
    if [[ ! -d "$experiment_dir" ]]; then
        continue
    fi

    num_traces=$(find $experiment_dir -maxdepth 1 -mindepth 1 -type d | wc -l)
    break
done

for ((i=1; i<=$num_traces; i++)); do
    first_experiment=true
    for experiment_dir in $results_dir/*; do
        if [[ ! -d "$experiment_dir" ]]; then
            continue
        fi

        if [[ "$first_experiment" == "true" ]]; then
            first_experiment=false

            echo ",trace_$i runtime (secs),,,,,,,,,,,,,,,,,,," >> $out_file        
            # Save to file heading with applications names.
            sed -n '3p' $experiment_dir/pods_trace_$i/runtimes_seconds.csv >> $out_file
        fi

        sed -n '4p' $experiment_dir/pods_trace_$i/runtimes_seconds.csv >> $out_file
    done

    # Add some padding for readability.
    echo ",,,,,,,,,,,,,,,,,,,," >> $out_file
    echo ",,,,,,,,,,,,,,,,,,,," >> $out_file
done

echo "Written aggregate runtimes to file to $out_file"
