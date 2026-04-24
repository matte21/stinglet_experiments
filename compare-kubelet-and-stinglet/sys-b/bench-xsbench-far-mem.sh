#!/bin/bash

set -o nounset
set -o pipefail

readonly once_setup_file="cgroup-created"
readonly cg_root="/sys/fs/cgroup"
readonly tst_cg="$cg_root/xsbench"

if [[ ! -f $once_setup_file ]]; then
    # roughly 64 GiB
    readonly tot_bytes="68719476736"

    # roughly 59 GiB
    readonly fast_mem_bytes="63350767616"

    # roughly 5 GiB
    readonly cxl_mem_bytes="5368709120"

    echo "+cpuset" | sudo tee $cg_root/cgroup.subtree_control

    sudo mkdir $tst_cg

    echo "0-7" | sudo tee "$tst_cg/cpuset.cpus"
    echo "0,2" | sudo tee "$tst_cg/cpuset.mems"
    echo $tot_bytes | sudo tee "$tst_cg/memory.max"
    echo "0 $fast_mem_bytes" | sudo tee "$tst_cg/memory.max_per_node"
    echo "2 $cxl_mem_bytes" | sudo tee "$tst_cg/memory.max_per_node"

    touch $once_setup_file
fi

for i in {1..2}; do
    sudo sh -c 'echo $$ > /sys/fs/cgroup/xsbench/cgroup.procs && exec /usr/bin/time -f "%e" ./xsbench -t 8 -g 130000 -p 30000000' 2>> time-measurements.txt
done
