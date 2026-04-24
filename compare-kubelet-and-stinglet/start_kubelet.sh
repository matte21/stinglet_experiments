#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

if [[ $# -ne 3 ]]; then
    echo "Invalid arguments!"
    echo "Usage: $0 <name of kubelet bin file> <name of kubelet config file> <logs folder>"
    exit 1
fi
readonly bin="kubelet-bins/$1"
readonly cfg_append="kubelet-cfgs/$2"
readonly logs_folder="$3"

if [[ ! -f "$bin" ]]; then
    echo "Fatal: kubelet bin file $1 does not exist under folder kubelet-bins/"
    exit 1
fi

if [[ ! -f "$cfg_append" ]]; then
    echo "Fatal: kubelet config file $2 does not exist under folder kubelet-cfgs/"
    exit 1
fi

readonly kubelet_cfg_file="/etc/kubernetes/kubelet.yaml"

# Ensure that in this experiment the kubelet starts with a clean state.
sudo rm -rf $kubelet_cfg_file
sudo rm -rf /var/lib/kubelet/*
sudo rm -rf /etc/kubernetes/manifests/*

# Prepare the kubelet config for this experiment, by appending the config file in the invocation
# arguments to a base config file (we split them to have smaller config files, the base config
# is the one we always need).
sudo cp kubelet-cfgs/kubelet_base_cfg.yaml $kubelet_cfg_file
cat $cfg_append | sudo tee -a $kubelet_cfg_file > /dev/null

sudo cp "$bin" /usr/bin/kubelet
sudo mkdir -p /etc/kubernetes/manifests

# Set up background process to collect kubelet logs.
mkdir -p $logs_folder
journalctl -f -u kubelet > $logs_folder/kubelet.log &
echo $! > kubelet-log-collector-pid

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
