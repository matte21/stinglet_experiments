#!/bin/bash

# This file holds environment vars for configuring the scripts.

set -o errexit
set -o nounset
set -o pipefail

# Exit early if this script was already sourced. Note that because the variables are readonly, if
# we don't exit earlier it's an error: bash yells at us.
if [[ -n "${env_sourced:-}" ]]; then
    return 0
fi
readonly env_sourced=1

# The following variables configure the building of kubelet and stinglet.
readonly k8s_repo="https://github.com/matte21/private-k8s"
readonly k8s_src_dir="$HOME/k8s"
readonly kubelet_bins_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/kubelet-bins"
readonly k8s_branches_to_test="v1.33.0-std v1.33.0++ stinglet"
