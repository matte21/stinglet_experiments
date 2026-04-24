#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

readonly dataset_path="../../volumes/kdd12"

readonly time_cmd=(/usr/bin/time -f "%e")

## single NUMA, isolated.

# 2 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-1 ./liblinear-multicore-train -m 2 -s 6 $dataset_path 2>&1 >> res-base-ll
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0,64 ./liblinear-multicore-train -m 2 -s 6 $dataset_path 2>&1 >> res-base-ll
done

# 4 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-3 ./liblinear-multicore-train -m 4 -s 6 $dataset_path 2>&1 >> res-base-ll
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0,1,64,65 ./liblinear-multicore-train -m 4 -s 6 $dataset_path 2>&1 >> res-base-ll 
done

# 6 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-5 ./liblinear-multicore-train -m 6 -s 6 $dataset_path 2>&1 >> res-base-ll
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-2,64-66 ./liblinear-multicore-train -m 6 -s 6 $dataset_path 2>&1 >> res-base-ll 
done

# 8 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-7 ./liblinear-multicore-train -m 8 -s 6 $dataset_path 2>&1 >> res-base-ll
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-3,64-67 ./liblinear-multicore-train -m 8 -s 6 $dataset_path 2>&1 >> res-base-ll 
done

## multiple NUMAs, isolated

# 4 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -C 0-1,32-33 ./liblinear-multicore-train -m 4 -s 6 $dataset_path 2>&1 >> res-diff-numas-ll
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -C 0,64,32,96 ./liblinear-multicore-train -m 4 -s 6 $dataset_path 2>&1 >> res-diff-numas-ll
done

# 6 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -C 0-2,32-34 ./liblinear-multicore-train -m 4 -s 6 $dataset_path 2>&1 >> res-diff-numas-ll
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -C 0,64,1,65,32,96 ./liblinear-multicore-train -m 4 -s 6 $dataset_path 2>&1 >> res-diff-numas-ll
done

# 8 spread and pack
for i in {1..3}; do
    "${time_cmd[@]}" numactl -C 0-3,32-35 ./liblinear-multicore-train -m 8 -s 6 $dataset_path 2>&1 >> res-diff-numas-ll
done
for i in {1..3}; do
    "${time_cmd[@]}" numactl -C 0,64,1,65,32,96,33,97 ./liblinear-multicore-train -m 8 -s 6 $dataset_path 2>&1 >> res-diff-numas-ll
done

## single NUMAs, collocated, always spread

# 2 cpus
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-1 ./liblinear-multicore-train -m 2 -s 6 $dataset_path 2>&1 >> res-col-ll-1 &
    "${time_cmd[@]}" numactl -m 0 -C 64-65 ./liblinear-multicore-train -m 2 -s 6 $dataset_path 2>&1 >> res-col-ll-2 &
    wait
done

# 4 cpus
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-3 ./liblinear-multicore-train -m 4 -s 6 $dataset_path 2>&1 >> res-col-ll-1 &
    "${time_cmd[@]}" numactl -m 0 -C 64-67 ./liblinear-multicore-train -m 4 -s 6 $dataset_path 2>&1 >> res-col-ll-2 &
    wait
done

# 6 cpus
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-5 ./liblinear-multicore-train -m 6 -s 6 $dataset_path 2>&1 >> res-col-ll-1 &
    "${time_cmd[@]}" numactl -m 0 -C 64-69 ./liblinear-multicore-train -m 6 -s 6 $dataset_path 2>&1 >> res-col-ll-2 &
    wait
done

# 8 cpus
for i in {1..3}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-7 ./liblinear-multicore-train -m 8 -s 6 $dataset_path 2>&1 >> res-col-ll-1 &
    "${time_cmd[@]}" numactl -m 0 -C 64-71 ./liblinear-multicore-train -m 8 -s 6 $dataset_path 2>&1 >> res-col-ll-2 &
    wait
done
