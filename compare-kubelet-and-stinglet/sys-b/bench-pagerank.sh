#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

readonly pr_args="-f ../../volumes/twitter.sg -i1000 -t1e-4 -n20"

readonly time_cmd=(/usr/bin/time -f "%e")

## single NUMA, isolated.

# 8 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-7 ./pagerank $pr_args 2>&1 >> res-base-pr
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-3,64-67 ./pagerank $pr_args 2>&1 >> res-base-pr
done

# 12 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-11 ./pagerank $pr_args 2>&1 >> res-base-pr
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-5,64-69 ./pagerank $pr_args 2>&1 >> res-base-pr
done

# 24 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-23 ./pagerank $pr_args 2>&1 >> res-base-pr
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-11,64-75 ./pagerank $pr_args 2>&1 >> res-base-pr
done

# 32 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-31 ./pagerank $pr_args 2>&1 >> res-base-pr
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-15,64-79 ./pagerank $pr_args 2>&1 >> res-base-pr
done

## multiple NUMAs, isolated

# 8 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -C 0-3,32-35 ./pagerank $pr_args 2>&1 >> res-diff-numas-pr
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -C 0-1,64-65,32-33,96-97 ./pagerank $pr_args 2>&1 >> res-diff-numas-pr
done

# 12 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -C 0-5,32-37 ./pagerank $pr_args 2>&1 >> res-diff-numas-pr
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -C 0-2,64-66,32-34,96-98 ./pagerank $pr_args 2>&1 >> res-diff-numas-pr
done

# 24 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -C 0-11,32-43 ./pagerank $pr_args 2>&1 >> res-diff-numas-pr
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -C 0-5,64-69,32-37,96-101 ./pagerank $pr_args 2>&1 >> res-diff-numas-pr
done

# 32 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -C 0-15,32-47 ./pagerank $pr_args 2>&1 >> res-diff-numas-pr
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -C 0-7,64-71,32-39,96-103 ./pagerank $pr_args 2>&1 >> res-diff-numas-pr
done

## single NUMAs, collocated, always spread

# 8 cpus
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-7 ./pagerank $pr_args 2>&1 >> res-col-pr-1 &
    "${time_cmd[@]}" numactl -m 0 -C 64-71 ./pagerank $pr_args 2>&1 >> res-col-pr-2 &
    wait
done

# 12 cpus
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-11 ./pagerank $pr_args 2>&1 >> res-col-pr-1 &
    "${time_cmd[@]}" numactl -m 0 -C 64-75 ./pagerank $pr_args 2>&1 >> res-col-pr-2 &
    wait
done

# 24 cpus
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-23 ./pagerank $pr_args 2>&1 >> res-col-pr-1 &
    "${time_cmd[@]}" numactl -m 0 -C 64-87 ./pagerank $pr_args 2>&1 >> res-col-pr-2 &
    wait
done

# 32 cpus
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-31 ./pagerank $pr_args 2>&1 >> res-col-pr-1 &
    "${time_cmd[@]}" numactl -m 0 -C 64-95 ./pagerank $pr_args 2>&1 >> res-col-pr-2 &
    wait
done
