#!/bin/bash

set -o errexit
set -o pipefail
set -o nounset

apt-get update -y && apt-get install -y numactl

cd ~
if ! command -v go &>/dev/null; then
    wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
    sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz
    rm go1.25.0.linux-amd64.tar.gz
    echo "export PATH=\$PATH:/usr/local/go/bin" >>  ~/.bashrc
    export PATH=$PATH:/usr/local/go/bin
    go version
fi

if [[ ! -d k8s ]]; then
    git clone https://github.com/matte21/private-k8s k8s
fi

cd stinglet_experiments/experiments/simulation
if [[ ! -f packing_trace_zone_a_v1.sqlite ]]; then
    ./prepare_azure_trace.sh
fi

cd trace_player_v2
make K8S_DIR="/users/molivi3/k8s"
