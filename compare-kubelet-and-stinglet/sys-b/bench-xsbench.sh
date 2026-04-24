#!/bin/bash

set -o nounset
set -o pipefail

readonly xsb_args="-g 130000 -p 30000000"

time_cmd() {
    /usr/bin/time -f "%e" "$@"
}

## single NUMA, isolated.

# 12 spread and pack
for i in {1..3}; do
    time_cmd numactl -m 0 -C 0-11 ./xsbench -t 12 $xsb_args >> res-base
done
for i in {1..3}; do
    time_cmd numactl -m 0 -C 0-5,64-69 ./xsbench -t 12 $xsb_args >> res-base
done

# 20 spread and pack
for i in {1..3}; do
    time_cmd numactl -m 0 -C 0-19 ./xsbench -t 20 $xsb_args >> res-base
done
for i in {1..3}; do
    time_cmd numactl -m 0 -C 0-9,64-73 ./xsbench -t 20 $xsb_args >> res-base
done

# 24 spread and pack
for i in {1..3}; do
    time_cmd numactl -m 0 -C 0-23 ./xsbench -t 24 $xsb_args >> res-base
done
for i in {1..3}; do
    time_cmd numactl -m 0 -C 0-11,64-75 ./xsbench -t 24 $xsb_args >> res-base
done

# 32 spread and pack
for i in {1..3}; do
    time_cmd numactl -m 0 -C 0-31 ./xsbench -t 32 $xsb_args >> res-base
done
for i in {1..3}; do
    time_cmd numactl -m 0 -C 0-15,64-79 ./xsbench -t 32 $xsb_args >> res-base
done

## multiple NUMAs, isolated

# 12 spread and pack
for i in {1..3}; do
    time_cmd numactl -C 0-5,32-37 ./xsbench -t 12 $xsb_args >> res-diff
done
for i in {1..3}; do
    time_cmd numactl -C 0-2,64-66,32-34,96-98 ./xsbench -t 12 $xsb_args >> res-diff 
done

# 20 spread and pack
for i in {1..3}; do
    time_cmd numactl -C 0-9,32-41 ./xsbench -t 20 $xsb_args >> res-diff
done
for i in {1..3}; do
    time_cmd numactl -C 0-5,64-69,32-35,96-99 ./xsbench -t 20 $xsb_args >> res-diff 
done

# 24 spread and pack
for i in {1..3}; do
    time_cmd numactl -C 0-11,32-43 ./xsbench -t 24 $xsb_args >> res-diff
done
for i in {1..3}; do
    time_cmd numactl -C 0-5,64-69,32-37,96-101 ./xsbench -t 24 $xsb_args >> res-diff 
done

# 32 spread and pack
for i in {1..3}; do
    time_cmd numactl -C 0-15,32-47 ./xsbench -t 32 $xsb_args >> res-diff
done
for i in {1..3}; do
    time_cmd numactl -C 0-7,64-71,32-39,96-103 ./xsbench -t 32 $xsb_args >> res-diff 
done

## single NUMAs, collocated, always spread

# 12 spread and pack
for i in {1..3}; do
    time_cmd numactl -m 0 -C 0-11 ./xsbench -t 12 $xsb_args >> res-col-1
    time_cmd numactl -m 0 -C 64-75 ./xsbench -t 12 $xsb_args >> res-col-2
    wait
done

# 20 spread and pack
for i in {1..3}; do
    time_cmd numactl -m 0 -C 0-19 ./xsbench -t 20 $xsb_args >> res-col-1
    time_cmd numactl -m 0 -C 64-83 ./xsbench -t 20 $xsb_args >> res-col-2 
    wait
done

# 24 spread and pack
for i in {1..3}; do
    time_cmd numactl -m 0 -C 0-23 ./xsbench -t 24 $xsb_args >> res-col-1
    time_cmd numactl -m 0 -C 64-87 ./xsbench -t 24 $xsb_args >> res-col-2 
    wait
done

# 32 spread and pack
for i in {1..3}; do
    time_cmd numactl -m 0 -C 0-31 ./xsbench -t 32 $xsb_args >> res-col-1
    time_cmd numactl -m 0 -C 64-95 ./xsbench -t 32 $xsb_args >> res-col-2 
    wait
done
