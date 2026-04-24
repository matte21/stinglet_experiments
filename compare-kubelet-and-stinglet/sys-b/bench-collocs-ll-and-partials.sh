#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

readonly pr_args="-f ../volumes/twitter.sg -i1000 -t1e-4 -n20"
readonly xsb_args="-g 130000 -p 30000000"
readonly ll_args="-m 6 -s 6 ../volumes/kdd12"

readonly time_cmd=(/usr/bin/time -f "%e")

# ll - xsb full.
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-5 ./liblinear-multicore-train $ll_args 2>&1 >> ll-xsb-full-ll &
    numactl -m 0 -C 64-83 ./xsb -t 20 $xsb_args 2>&1 >> ll-xsb-full-xsb &
    wait
    sleep 2
done

# ll - xsb partial.
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-5 ./liblinear-multicore-train $ll_args 2>&1 >> ll-xsb-p-ll &
    numactl -m 0 -C 67-86 ./xsb -t 20 $xsb_args 2>&1 >> ll-xsb-p-xsb &
    wait
    sleep 2
done

# ll - pr full.
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-5 ./liblinear-multicore-train $ll_args 2>&1 >> ll-pr-full-ll &
    numactl -m 0 -C 64-81 ./pr $pr_args 2>&1 >> ll-pr-full-pr &
    wait
    sleep 2
done

# ll - pr partial.
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-5 ./liblinear-multicore-train $ll_args 2>&1 >> ll-pr-p-ll &
    numactl -m 0 -C 67-84 ./pr $pr_args 2>&1 >> ll-pr-p-pr &
    wait
    sleep 2
done

# ll - ll partial.
for i in {1..2}; do
    "${time_cmd[@]}" numactl -m 0 -C 0-5 ./liblinear-multicore-train $ll_args 2>&1 >> ll-ll-p-ll-1 &
    numactl -m 0 -C 67-72 ./liblinear-multicore-train $ll_args 2>&1 >> ll-ll-p-ll-2 &
    wait
    sleep 2
done
