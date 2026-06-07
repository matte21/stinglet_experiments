#!/bin/bash

set -o errexit
set -o pipefail
set -o nounset

if [[ $# -lt 1 ]]; then
    echo "mid argument is required but wasn't provided"
    exit 1
fi
readonly mid="$1"

readonly res_dir="results/$(date +%Y-%m-%d-%H-%M-%S)"
mkdir -p "$res_dir"

bin/sim-stinglet --trace-db-file "../packing_trace_zone_a_v1.sqlite" \
    --config-file "configs/$mid-s4-stinglet.cfg.yaml" \
    --num-workers 3 \
    --output-dir "$res_dir" \
    --output-file-prefix "stinglet" 2>&1 | grep --line-buffered "SIM:" > stinglet.log &
stinglet_pid=$!

bin/sim-v1.33.0-std --trace-db-file "../packing_trace_zone_a_v1.sqlite" \
    --config-file "configs/$mid-s4-kubelet.cfg.yaml" \
    --num-workers 5 \
    --output-dir "$res_dir" \
    --output-file-prefix "kubelet" 2>&1 | grep --line-buffered "SIM:" > kubelet.log &
kubelet_pid=$!

bin/sim-v1.33.0++ --trace-db-file "../packing_trace_zone_a_v1.sqlite" \
    --config-file "configs/$mid-s4-kubelet++.cfg.yaml" \
    --num-workers 5 \
    --output-dir "$res_dir" \
    --output-file-prefix "kubelet++" 2>&1 | grep --line-buffered "SIM:" > kubelet++.log &
kubelet_plus_pid=$!

wait $stinglet_pid || echo "sim-stinglet failed"
wait $kubelet_pid || echo "sim-v1.33.0-std failed"
wait $kubelet_plus_pid || echo "sim-v1.33.0++ failed"

cp "configs/$mid-s4-stinglet.cfg.yaml" "$res_dir"
cp "configs/$mid-s4-kubelet.cfg.yaml" "$res_dir"
cp "configs/$mid-s4-kubelet++.cfg.yaml" "$res_dir"

stinglet_res_la=$(awk '/^$/{exit} 1' "$res_dir/stinglet-$mid-LeastAllocated-s4.csv")
kubelet_res_la=$(awk '/^$/{exit} 1' "$res_dir/kubelet-$mid-LeastAllocated-s4.csv" | tail -n +2)
kubelet_plus_res_la=$(awk '/^$/{exit} 1' "$res_dir/kubelet++-$mid-LeastAllocated-s4.csv" | tail -n +2)

echo "$stinglet_res_la" > "$res_dir/summary-$mid-LeastAllocated-s4.csv"
echo "$kubelet_res_la" >> "$res_dir/summary-$mid-LeastAllocated-s4.csv"
echo "$kubelet_plus_res_la" >> "$res_dir/summary-$mid-LeastAllocated-s4.csv"

stinglet_res_ma=$(awk '/^$/{exit} 1' "$res_dir/stinglet-$mid-MostAllocated-s4.csv")
kubelet_res_ma=$(awk '/^$/{exit} 1' "$res_dir/kubelet-$mid-MostAllocated-s4.csv" | tail -n +2)
kubelet_plus_res_ma=$(awk '/^$/{exit} 1' "$res_dir/kubelet++-$mid-MostAllocated-s4.csv" | tail -n +2)

echo "$stinglet_res_ma" > "$res_dir/summary-$mid-MostAllocated-s4.csv"
echo "$kubelet_res_ma" >> "$res_dir/summary-$mid-MostAllocated-s4.csv"
echo "$kubelet_plus_res_ma" >> "$res_dir/summary-$mid-MostAllocated-s4.csv"

# vlab version

stinglet_res_la_vlab=$(awk '/^$/{exit} 1' "$res_dir/stinglet-$mid-LeastAllocated-vlab.csv")
kubelet_res_la_vlab=$(awk '/^$/{exit} 1' "$res_dir/kubelet-$mid-LeastAllocated-vlab.csv" | tail -n +2)
kubelet_plus_res_la_vlab=$(awk '/^$/{exit} 1' "$res_dir/kubelet++-$mid-LeastAllocated-vlab.csv" | tail -n +2)

echo "$stinglet_res_la_vlab" > "$res_dir/summary-$mid-LeastAllocated-vlab.csv"
echo "$kubelet_res_la_vlab" >> "$res_dir/summary-$mid-LeastAllocated-vlab.csv"
echo "$kubelet_plus_res_la_vlab" >> "$res_dir/summary-$mid-LeastAllocated-vlab.csv"

stinglet_res_ma_vlab=$(awk '/^$/{exit} 1' "$res_dir/stinglet-$mid-MostAllocated-vlab.csv")
kubelet_res_ma_vlab=$(awk '/^$/{exit} 1' "$res_dir/kubelet-$mid-MostAllocated-vlab.csv" | tail -n +2)
kubelet_plus_res_ma_vlab=$(awk '/^$/{exit} 1' "$res_dir/kubelet++-$mid-MostAllocated-vlab.csv" | tail -n +2)

echo "$stinglet_res_ma_vlab" > "$res_dir/summary-$mid-MostAllocated-vlab.csv"
echo "$kubelet_res_ma_vlab" >> "$res_dir/summary-$mid-MostAllocated-vlab.csv"
echo "$kubelet_plus_res_ma_vlab" >> "$res_dir/summary-$mid-MostAllocated-vlab.csv"
