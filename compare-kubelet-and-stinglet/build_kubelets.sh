#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

if [ $# -lt 2 ]; then
    echo "Invalid arguments"
    echo "Usage: $0 <path to k8s src dir> <k8s git branch/tag 1> [<k8s git branch/tag 2> ... <k8s git branch/tag N>]"
    exit 1
fi
readonly k8s_src_dir="$1"
shift

readonly kubelet_bins_dir="`pwd`/kubelet-bins"
mkdir -p $kubelet_bins_dir

if [ ! -d $k8s_src_dir ]; then
    echo "Fatal: k8s source dir $k8s_src_dir does not exist"
    exit 1
fi
cd $k8s_src_dir

# git checkout and build each branch individually, move them to the folder where the
# script to run the experiments expects them to be.  
while (( "$#" )); do
    git checkout $1 || { echo "Failed to git checkout branch $1 in $k8s_src_dir !"; exit 1; }
    make test WHAT="pkg/kubelet/... cmd/kubelet/..." build kubelet || { echo "Testing and building kubelet in branch $1 failed!"; exit 1; }

    sudo mv "$k8s_src_dir/_output/bin/kubelet" "$kubelet_bins_dir/$1"
    chmod +x "$kubelet_bins_dir/$1"

    shift
done
