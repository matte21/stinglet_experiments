#!/bin/bash

set -o errexit

systemctl stop kubelet

rm -rf /var/lib/kubelet/*
rm -rf /etc/kubernetes/kubelet.yaml
