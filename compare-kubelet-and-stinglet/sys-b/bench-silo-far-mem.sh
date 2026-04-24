#!/bin/bash

set -o nounset
set -o pipefail
set -o errexit

readonly once_setup_file="cgroup-created"
readonly cg_root="/sys/fs/cgroup"
readonly tst_cg="$cg_root/silo-test"

if [[ ! -f $once_setup_file ]]; then
    echo "+cpuset +memory" | sudo tee "$cg_root/cgroup.subtree_control"
    sudo mkdir "$tst_cg"

    echo "0,2" | sudo tee "$tst_cg/cpuset.mems"
    echo "0-7" | sudo tee "$tst_cg/cpuset.cpus"

    # 29 GiB
    readonly tot_bytes="31138512896"
    echo $tot_bytes | sudo tee "$tst_cg/memory.max"

    touch $once_setup_file
fi

# baseline run: no per-node limits (all local).
for i in {1..2}; do
    echo "starting baseline run $i"
    sudo sh -c 'echo $$ > /sys/fs/cgroup/silo-test/cgroup.procs && exec ./dbtest --verbose --bench ycsb --scale-factor 200000  --num-threads 8 --ops-per-worker 400000000' 2>> silo-measurements.txt
done

# roughly 27 GiB on local mem and 2 GiB on CXL mem.
fast_mem_bytes="28991029248"
cxl_mem_bytes="2147483648"
echo "0 $fast_mem_bytes" | sudo tee "$tst_cg/memory.max_per_node"
echo "2 $cxl_mem_bytes" | sudo tee "$tst_cg/memory.max_per_node"

for i in {1..2}; do
    echo "starting run 27-2 $i"
    sudo sh -c 'echo $$ > /sys/fs/cgroup/silo-test/cgroup.procs && exec ./dbtest --verbose --bench ycsb --scale-factor 200000  --num-threads 8 --ops-per-worker 400000000' 2>> silo-measurements.txt
done

# roughly 25 GiB on local mem and 4 GiB on CXL mem.
fast_mem_bytes="26843545600"
cxl_mem_bytes="4294967296"
echo "0 $fast_mem_bytes" | sudo tee "$tst_cg/memory.max_per_node"
echo "2 $cxl_mem_bytes" | sudo tee "$tst_cg/memory.max_per_node"
for i in {1..2}; do
    echo "starting run 25-4 $i"
    sudo sh -c 'echo $$ > /sys/fs/cgroup/silo-test/cgroup.procs && exec ./dbtest --verbose --bench ycsb --scale-factor 200000  --num-threads 8 --ops-per-worker 400000000' 2>> silo-measurements.txt
done

# roughly 23 GiB on local mem and 6 GiB on CXL mem.
fast_mem_bytes="24696061952"
cxl_mem_bytes="6442450944"
echo "0 $fast_mem_bytes" | sudo tee "$tst_cg/memory.max_per_node"
echo "2 $cxl_mem_bytes" | sudo tee "$tst_cg/memory.max_per_node"
for i in {1..2}; do
    echo "starting run 23-6 $i"
    sudo sh -c 'echo $$ > /sys/fs/cgroup/silo-test/cgroup.procs && exec ./dbtest --verbose --bench ycsb --scale-factor 200000  --num-threads 8 --ops-per-worker 400000000' 2>> silo-measurements.txt
done

# roughly 21 GiB on local mem and 8 GiB on CXL mem.
fast_mem_bytes="22548578304"
cxl_mem_bytes="8589934592"
echo "0 $fast_mem_bytes" | sudo tee "$tst_cg/memory.max_per_node"
echo "2 $cxl_mem_bytes" | sudo tee "$tst_cg/memory.max_per_node"
for i in {1..2}; do
    echo "starting run 21-8 $i"
    sudo sh -c 'echo $$ > /sys/fs/cgroup/silo-test/cgroup.procs && exec ./dbtest --verbose --bench ycsb --scale-factor 200000  --num-threads 8 --ops-per-worker 400000000' 2>> silo-measurements.txt
done

# roughly 17 GiB on local mem and 12 GiB on CXL mem.
fast_mem_bytes="18253611008"
cxl_mem_bytes="12884901888"
echo "0 $fast_mem_bytes" | sudo tee "$tst_cg/memory.max_per_node"
echo "2 $cxl_mem_bytes" | sudo tee "$tst_cg/memory.max_per_node"
for i in {1..2}; do
    echo "starting run 17-12 $i"
    sudo sh -c 'echo $$ > /sys/fs/cgroup/silo-test/cgroup.procs && exec ./dbtest --verbose --bench ycsb --scale-factor 200000  --num-threads 8 --ops-per-worker 400000000' 2>> silo-measurements.txt
done

echo "done"
