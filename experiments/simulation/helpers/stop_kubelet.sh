#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

sudo systemctl stop kubelet.service
while [[ "$(sudo systemctl is-active kubelet.service)" != "inactive" ]]; do
    sleep 1
    echo waiting for kubelet service to stop.
done
echo Stopped kubelet.

# Ensure that the next time the kubelet starts it does so with a clean state.
# We're already doing it before starting it, but doing it again protects attempts to
# start the kubelet that don't use this script (or future-proofs this against me modifying
# the start script by removing this safeguard).
sudo rm -f /etc/kubernetes/kubelet.yaml
sudo rm -rf /var/lib/kubelet/*
