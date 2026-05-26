#!/bin/bash

set -o errexit
set -o pipefail
set -o nounset

bin/sim-stinglet --trace-db-file "../packing_trace_zone_a_v1.sqlite" \
    --config-file "stinglet.cfg.yaml" \
    --num-workers 9 > stinglet.log 2>&1 &

bin/sim-v1.33.0-std --trace-db-file "../packing_trace_zone_a_v1.sqlite" \
    --config-file "kubelet.cfg.yaml" \
    --num-workers 15 > kubelet.log 2>&1 &

bin/sim-v1.33.0++ --trace-db-file "../packing_trace_zone_a_v1.sqlite" \
    --config-file "kubelet++.cfg.yaml" \
    --num-workers 15 > kubelet++.log 2>&1 &

wait
