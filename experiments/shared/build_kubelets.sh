#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

source env.sh

if [ ! -d "$k8s_src_dir" ]; then
    echo "Directory with k8s source code $k8s_src_dir doesn't exist: cloning k8s repo to it."
    if ! git clone "$k8s_repo" "$k8s_src_dir"; then
        echo "Fatal: failed to clone k8s; removing clone directory for idempotency."
        rm -rf "$k8s_src_dir"
        exit 1
    fi
fi

mkdir -p "$kubelet_bins_dir"
cd "$k8s_src_dir"

# git checkout and build each branch individually, move them to the folder where the script to run
# the experiments expects them to be.  
for b in $k8s_branches_to_test; do
    echo "building branch $b"

    git checkout "$b" || { echo "Failed to git checkout branch $b in $k8s_src_dir !"; exit 1; }

    # Branches other than the standard v1.33.0 might fail because we intentionally break kubelet's
    # expected behavior.
    if [[ "$b" == "v1.33.0-std" ]]; then
        make test WHAT="pkg/kubelet/... cmd/kubelet/..."  || { echo "Fatal: testing kubelet branch $b failed, should never happen."; exit 1;}
    else
        make test WHAT="pkg/kubelet/... cmd/kubelet/..."  || { echo "Testing kubelet branch $b failed AS EXPECTED. If the only tests failing are for topology, cpu or memory manager it's expected. Otherwise, you have a bug."; }
    fi

    # "all" means that version information will be added as well
    make all WHAT=cmd/kubelet || { echo "Building kubelet branch $b failed!"; exit 1; }

    sudo mv "$k8s_src_dir/_output/bin/kubelet" "$kubelet_bins_dir/$b"
    chmod +x "$kubelet_bins_dir/$b"
done
