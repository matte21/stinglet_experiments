#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

sudo apt-get update -y
sudo apt-get install -y jq yq iptables nftables libnftnl-dev
sudo update-alternatives --set iptables /usr/sbin/iptables-nft
sudo update-alternatives --set ip6tables /usr/sbin/ip6tables-nft

#sudo modprobe overlay
#sudo modprobe ip_tables
#sudo modprobe nf_tables
#sudo modprobe xt_comment
#sudo modprobe veth
#sudo modprobe bridge

# ! crio config!
# [crio.image]
# signature_policy = "/etc/crio/policy.json"

# [crio.runtime]
# default_runtime = "crun"
# conmon_cgroup = "pod"
# cgroup_manager = "cgroupfs"

# [crio.runtime.runtimes.crun]
# runtime_path = "/users/molivi3/crun"
# runtime_root = "/run/crun"
# monitor_path = "/usr/libexec/crio/conmon"
# allowed_annotations = [
#     "io.containers.trace-syscall",
# ]

# [crio.runtime.runtimes.runc]
# runtime_path = "/usr/libexec/crio/runc"
# runtime_root = "/run/runc"
# monitor_path = "/usr/libexec/crio/conmon"
