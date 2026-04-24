#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

readonly time_cmd=(/usr/bin/time -f "%e")

readonly bwaves_bin="benchspec/CPU/603.bwaves_s/run/run_base_refspeed_mytest-m64.0000/speed_bwaves_base.mytest-m64"
readonly bwaves_in="benchspec/CPU/603.bwaves_s/run/run_base_refspeed_mytest-m64.0000/bwaves_1.in"

## bwaves, single NUMA.
# 4 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-3 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-1n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-1,64-65 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-1n
done

# 6 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-5 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-1n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-2,64-66 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-1n
done

# 12 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-11 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-1n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-5,64-69 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-1n 
done

# 18 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-17 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-1n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-8,64-72 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-1n 
done

# 24 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-17 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-1n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-8,64-72 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-1n 
done

## bwaves, two NUMAs.
# 4 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-1,32-33 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-2n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0,64,32,96 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-2n
done

# 6 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-2,32-34 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-2n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-1,64-65,32,96 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-2n
done

# 12 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-5,32-37 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-2n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-2,64-66,32-34,96-98 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-2n
done

# 18 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-8,32-40 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-2n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-4,64-68,32-35,96-99 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-2n
done

# 24 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-11,32-43 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-2n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-5,64-69,32-37,96-101 $bwaves_bin < $bwaves_in 2>&1 >> res-bwaves-2n
done

readonly roms_bin="benchspec/CPU/654.roms_s/run/run_base_refspeed_mytest-m64.0000/sroms_base.mytest-m64"
readonly roms_in="benchspec/CPU/654.roms_s/run/run_base_refspeed_mytest-m64.0000/ocean_benchmark3.in"

## roms, single NUMA
# 4 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-3 $roms_bin < $roms_in 2>&1 >> res-roms-1n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-1,64-65 $roms_bin < $roms_in 2>&1 >> res-roms-1n
done

# 8 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-7 $roms_bin < $roms_in 2>&1 >> res-roms-1n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-3,64-67 $roms_bin < $roms_in 2>&1 >> res-roms-1n
done

# 16 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-15 $roms_bin < $roms_in 2>&1 >> res-roms-1n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-7,64-71 $roms_bin < $roms_in 2>&1 >> res-roms-1n 
done

# 32 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-31 $roms_bin < $roms_in 2>&1 >> res-roms-1n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-15,64-79 $roms_bin < $roms_in 2>&1 >> res-roms-1n 
done

## roms, two NUMAs.
# 4 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-1,32-33 $roms_bin < $roms_in 2>&1 >> res-roms-1n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0,64,32,96 $roms_bin < $roms_in 2>&1 >> res-roms-1n
done

# 8 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-3,32-35 $roms_bin < $roms_in 2>&1 >> res-roms-2n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-1,64-65,32-33,96-97 $roms_bin < $roms_in 2>&1 >> res-roms-2n
done

# 16 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-7,32-39 $roms_bin < $roms_in 2>&1 >> res-roms-2n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-3,64-67,32-35,96-99 $roms_bin < $roms_in 2>&1 >> res-roms-2n
done

# 32 spread and pack
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-15,32-47 $roms_bin < $roms_in 2>&1 >> res-roms-2n
done
for i in {1..2}; do
    "${time_cmd[@]}" numactl -C 0-7,64-71,32-39,96-103 $roms_bin < $roms_in 2>&1 >> res-roms-2n
done
