#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

readonly pr_args="-f ../../volumes/twitter.sg -i1000 -t1e-4 -n20"
readonly xsb_args="-g 130000 -p 30000000"

readonly time_cmd=(/usr/bin/time -f "%e")

# pr - xsb.
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-17 ./pagerank $pr_args 2>&1 >> pr-xsb-pr &
    "${time_cmd[@]}" numactl -m 0 -C 64-83 ./xsbench -t 20 $xsb_args 2>&1 >> pr-xsb-xsb &
    wait
    sleep 2
done

# pr - bt.
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-17 ./pagerank $pr_args 2>&1 >> pr-bt-pr &
    "${time_cmd[@]}" numactl -m 0 -C 64-87 ./bench_btree_mt 2>&1 >> pr-bt-bt &
    wait
    sleep 2
done
