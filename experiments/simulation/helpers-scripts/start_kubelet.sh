#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

source ../../shared/env.sh

if [[ $# -ne 2 ]]; then
    echo "Invalid arguments!"
    echo "Usage: $0 <name of kubelet bin file> <name of kubelet config file>"
    exit 1
fi
readonly bin="$1"
readonly cfg_append="$2"

readonly kubelet_cfg_file="/etc/kubernetes/kubelet.yaml"

# Ensure that in this experiment the kubelet starts with a clean state.
sudo rm -rf "$kubelet_cfg_file"
sudo rm -rf /var/lib/kubelet/*
sudo rm -rf /etc/kubernetes/manifests/*

# Prepare the kubelet config for this experiment, by appending the config file in the invocation
# arguments to a base config file (we split them to have smaller config files, the base config
# is the one we always need).
sudo cp "$kubelet_cfgs_dir/kubelet_base_cfg.yaml" "$kubelet_cfg_file"
cat "$cfg_append" | sudo tee -a "$kubelet_cfg_file" > /dev/null

sudo cp "$bin" /usr/bin/kubelet
sudo mkdir -p /etc/kubernetes/manifests

# Now, start the kubelet.
sudo systemctl start kubelet.service
while [[ "$(sudo systemctl is-active kubelet.service)" != "active" ]]; do
    sleep 1
    echo waiting for kubelet service to get up and running.
done

# Wait until the the kubelet is up and running and able to handle pods.
while [[ "$(curl -s http://localhost:10255/healthz)" != "ok" || -z "$(curl -s http://localhost:10255/pods | grep kind)" ]]; do
    sleep 1
    echo Waiting for kubelet to be able to handle pods...
done
