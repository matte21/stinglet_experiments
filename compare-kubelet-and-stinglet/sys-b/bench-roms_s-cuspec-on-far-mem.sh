#!/bin/bash

set -o errexit
set -o pipefail
set -o nounset

readonly once_setup_file="cgroup-created"
readonly cg_root="/sys/fs/cgroup"
readonly tst_cg="$cg_root/cpuspec-roms"

if [[ ! -f $once_setup_file ]]; then
    # roughly 10.2 GiB
    readonly tot_bytes="10952376320"

    # roughly 8.2 GiB
    readonly fast_mem_bytes="8804892672"

    # roughly 2 GiB
    readonly cxl_mem_bytes="2147483648"

    echo "+cpuset" | sudo tee $cg_root/cgroup.subtree_control

    sudo mkdir $tst_cg

    echo "0-7" | sudo tee "$tst_cg/cpuset.cpus"
    echo "0,2" | sudo tee "$tst_cg/cpuset.mems"
    echo $tot_bytes | sudo tee "$tst_cg/memory.max"
    echo "0 $fast_mem_bytes" | sudo tee "$tst_cg/memory.max_per_node"
    echo "2 $cxl_mem_bytes" | sudo tee "$tst_cg/memory.max_per_node"

    touch $once_setup_file
fi

for i in {1..3}; do
    sudo sh -c 'echo $$ > /sys/fs/cgroup/cpuspec-roms/cgroup.procs && exec /usr/bin/time -f "%e" ./sroms_base.mytest-m64 < ocean_benchmark3.in' 2>> time-measurements.txt
done
