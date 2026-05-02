#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

source ../env.sh

if [[ -z "${1:-}" ]]; then
    echo "Fatal: invalid arguments."
    echo "Usage: $0 <file name of experiment description file in dir \"experiments_descriptions\">"
    exit 1
fi
readonly experiment_file="$1"

# Start the container runtime, CRI-O.
sudo systemctl start crio.service
while [[ "$(sudo systemctl is-active crio.service)" != "active" ]]; do
    sleep 1
    echo waiting for cri-o service to get up and running.
done

# Prepare folder where we'll store experiments data.
readonly output_folder="collected-data/$(date +'%Y-%m-%d-%H-%M-%S')"
mkdir -p "$output_folder"
cp experiments_descriptions/$experiment_file $output_folder
