#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

if [[ "$#" -ne 1 ]]; then
    echo "Invalid arguments"
    echo "Usage: $0 <path to folder with baseline results>"
    exit 1
fi
readonly baselines_dir="$1"

if [[ ! -d "$baselines_dir" ]]; then
    echo "Fatal: folder $baselines_dir doesn't exist."
    exit 1
fi

readonly out_file="$(basename $baselines_dir | cut -d '-' -f1).csv"

echo ",runtime secs,," > $out_file

for experiment_dir in $baselines_dir/*; do    
    if [[ ! -d "$experiment_dir" ]]; then
        continue
    fi

    for trace in $experiment_dir/*; do
        if [[ ! -d "$trace" ]]; then
            continue
        fi

        experiment=$(basename $experiment_dir)
        
        runtime_secs=$(grep $experiment $trace/runtimes_seconds.csv | cut -d ',' -f 2)

        echo "$experiment,$runtime_secs,," >> $out_file
    done
done

echo "Written aggregate baselines to file to $out_file"
