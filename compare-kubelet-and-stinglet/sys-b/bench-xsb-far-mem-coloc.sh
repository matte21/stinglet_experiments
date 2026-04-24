#!/bin/bash

set -o nounset
set -o pipefail

readonly once_setup_file="cgroup-created"
readonly cg_root="/sys/fs/cgroup"
readonly tst_cg_1="$cg_root/xsbench-1"
readonly tst_cg_2="$cg_root/xsbench-2"

if [[ ! -f $once_setup_file ]]; then
    # roughly 64 GiB
    readonly tot_bytes="68719476736"

    # roughly 59 GiB
    readonly fast_mem_bytes="63350767616"

    # roughly 5 GiB
    readonly cxl_mem_bytes="5368709120"

    echo "+cpuset" | sudo tee $cg_root/cgroup.subtree_control

    sudo mkdir $tst_cg_1
    sudo mkdir $tst_cg_2

    echo "0-7" | sudo tee "$tst_cg_1/cpuset.cpus"
    echo "2,4" | sudo tee "$tst_cg_1/cpuset.mems"
    echo $tot_bytes | sudo tee "$tst_cg_1/memory.max"
    echo "2 $fast_mem_bytes" | sudo tee "$tst_cg_1/memory.max_per_node"
    echo "4 $cxl_mem_bytes" | sudo tee "$tst_cg_1/memory.max_per_node"

    echo "8-15" | sudo tee "$tst_cg_2/cpuset.cpus"
    echo "2,4" | sudo tee "$tst_cg_2/cpuset.mems"
    echo $tot_bytes | sudo tee "$tst_cg_2/memory.max"
    echo "2 $fast_mem_bytes" | sudo tee "$tst_cg_2/memory.max_per_node"
    echo "4 $cxl_mem_bytes" | sudo tee "$tst_cg_2/memory.max_per_node"

    touch $once_setup_file
fi

sudo sh -c 'echo $$ > /sys/fs/cgroup/xsbench-1/cgroup.procs && exec /usr/bin/time -f "%e" ./xsbench -t 8 -g 130000 -p 30000000' >> time-measurements-1.txt 2>&1 &
sudo sh -c 'echo $$ > /sys/fs/cgroup/xsbench-2/cgroup.procs && exec /usr/bin/time -f "%e" ./xsbench -t 8 -g 130000 -p 30000000' >> time-measurements-2.txt 2>&1 &

wait
