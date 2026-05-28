#!/bin/bash

set -o errexit
set -o pipefail
set -o nounset

readonly res_dir="results/$(date +%Y-%m-%d-%H-%M-%S)"
mkdir -p "$res_dir"

bin/sim-stinglet --trace-db-file "../packing_trace_zone_a_v1.sqlite" \
    --config-file "16-stinglet.cfg.yaml" \
    --num-workers 4 \
    --results-dir "$res_dir" \
    --res-prefix "stinglet" 2>&1 | grep --line-buffered "SIM:" > stinglet.log &

bin/sim-v1.33.0-std --trace-db-file "../packing_trace_zone_a_v1.sqlite" \
    --config-file "16-kubelet.cfg.yaml" \
    --num-workers 6 \
    --results-dir "$res_dir" \
    --res-prefix "kubelet" 2>&1 | grep --line-buffered "SIM:" > kubelet.log &

bin/sim-v1.33.0++ --trace-db-file "../packing_trace_zone_a_v1.sqlite" \
    --config-file "16-kubelet++.cfg.yaml" \
    --num-workers 6 \
    --results-dir "$res_dir" \
    --res-prefix "kubelet++" 2>&1 | grep --line-buffered "SIM:" > kubelet++.log &

wait

cp "16-stinglet.cfg.yaml" "$res_dir"
cp "16-kubelet.cfg.yaml" "$res_dir"
cp "16-kubelet++.cfg.yaml" "$res_dir"

stinglet_res_la=$(awk '/^$/{exit} 1' "$res_dir/stinglet-16-LeastAllocated-s4.csv")
kubelet_res_la=$(awk '/^$/{exit} 1' "$res_dir/kubelet-16-LeastAllocated-s4.csv" | tail -n +2)
kubelet_plus_res_la=$(awk '/^$/{exit} 1' "$res_dir/kubelet++-16-LeastAllocated-s4.csv" | tail -n +2)

echo "$stinglet_res_la" > "$res_dir/summary-16-LeastAllocated-s4.csv"
echo "$kubelet_res_la" >> "$res_dir/summary-16-LeastAllocated-s4.csv"
echo "$kubelet_plus_res_la" >> "$res_dir/summary-16-LeastAllocated-s4.csv"

stinglet_res_ma=$(awk '/^$/{exit} 1' "$res_dir/stinglet-16-MostAllocated-s4.csv")
kubelet_res_ma=$(awk '/^$/{exit} 1' "$res_dir/kubelet-16-MostAllocated-s4.csv" | tail -n +2)
kubelet_plus_res_ma=$(awk '/^$/{exit} 1' "$res_dir/kubelet++-16-MostAllocated-s4.csv" | tail -n +2)

echo "$stinglet_res_ma" > "$res_dir/summary-16-MostAllocated-s4.csv"
echo "$kubelet_res_ma" >> "$res_dir/summary-16-MostAllocated-s4.csv"
echo "$kubelet_plus_res_ma" >> "$res_dir/summary-16-MostAllocated-s4.csv"
