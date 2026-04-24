#!/bin/bash

set -o errexit
set -o pipefail
set -o nounset

readonly base_cmd="./dbtest --verbose --bench ycsb --scale-factor 400000 --slow-exit --parallel-loading"

# We run the benchmark so that in total there are 400000000 * 8 = 3.2 billion ops. So as we change
# the number of workers we also change the number of ops per worker.

## single NUMA, isolated.

# 6 cpus spread and pack
for i in {1..3}; do
    numactl -m 0 -C 0-5 $base_cmd --num-threads 6 --ops-per-worker 533333334 2>&1 >> res-base-sl
done
for i in {1..3}; do
    numactl -m 0 -C 0-2,64-66 $base_cmd --num-threads 6 --ops-per-worker 533333334 2>&1 >> res-base-sl 
done

# 8 cpus spread and pack
for i in {1..3}; do
    numactl -m 0 -C 0-7 $base_cmd --num-threads 8 --ops-per-worker 400000000 2>&1 >> res-base-sl
done
for i in {1..3}; do
    numactl -m 0 -C 0-3,64-67 $base_cmd --num-threads 8 --ops-per-worker 400000000 2>&1 >> res-base-sl 
done

# 16 cpus spread and pack
for i in {1..3}; do
    numactl -m 0 -C 0-15 $base_cmd --num-threads 16 --ops-per-worker 200000000 2>&1 >> res-base-sl
done
for i in {1..3}; do
    numactl -m 0 -C 0-7,64-71 $base_cmd --num-threads 16 --ops-per-worker 200000000 2>&1 >> res-base-sl 
done

# 24 cpus spread and pack
for i in {1..3}; do
    numactl -m 0 -C 0-23 $base_cmd --num-threads 24 --ops-per-worker 133333334 2>&1 >> res-base-sl
done
for i in {1..3}; do
    numactl -m 0 -C 0-11,64-75 $base_cmd --num-threads 24 --ops-per-worker 133333334 2>&1 >> res-base-sl 
done

## multiple NUMAs, isolated

# 6 cpus spread and pack
for i in {1..3}; do
    numactl -C 0-2,32-34 $base_cmd --num-threads 6 --ops-per-worker 533333334 2>&1 >> res-diff-numas-sl
done
for i in {1..3}; do
    numactl -C 0,64,1,65,32,96 $base_cmd --num-threads 6 --ops-per-worker 533333334 2>&1 >> res-diff-numas-sl 
done

# 8 cpus spread and pack
for i in {1..3}; do
    numactl -C 0-3,32-35 $base_cmd --num-threads 8 --ops-per-worker 400000000 2>&1 >> res-diff-numas-sl
done
for i in {1..3}; do
    numactl -C 0,64,1,65,32,96,33,97 $base_cmd --num-threads 8 --ops-per-worker 400000000 2>&1 >> res-diff-numas-sl 
done

# 16 cpus spread and pack
for i in {1..3}; do
    numactl -C 0-7,32-39 $base_cmd --num-threads 16 --ops-per-worker 200000000 2>&1 >> res-diff-numas-sl
done
for i in {1..3}; do
    numactl -C 0-3,64-67,32-35,96-99 $base_cmd --num-threads 16 --ops-per-worker 200000000 2>&1 >> res-diff-numas-sl 
done

# 24 cpus spread and pack
for i in {1..3}; do
    numactl -C 0-11,32-43 $base_cmd --num-threads 24 --ops-per-worker 133333334 2>&1 >> res-diff-numas-sl
done
for i in {1..3}; do
    numactl -C 0-5,64-69,32-37,96-101 $base_cmd --num-threads 24 --ops-per-worker 133333334 2>&1 >> res-diff-numas-sl
done
