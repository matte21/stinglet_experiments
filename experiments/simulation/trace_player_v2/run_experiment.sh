#!/bin/bash

set -o errexit
set -o pipefail
set -o nounset

bin/sim-stinglet --trace-db-file "../packing_trace_zone_a_v1.sqlite" \
    --config-file "stinglet.cfg.yaml" \
    --num-workers 9 2>&1 | grep "SIM:" > stinglet.log &

bin/sim-v1.33.0-std --trace-db-file "../packing_trace_zone_a_v1.sqlite" \
    --config-file "kubelet.cfg.yaml" \
    --num-workers 15 2>&1 | grep "SIM:" > kubelet.log &

bin/sim-v1.33.0++ --trace-db-file "../packing_trace_zone_a_v1.sqlite" \
    --config-file "kubelet++.cfg.yaml" \
    --num-workers 15 2>&1 | grep "SIM:" > kubelet++.log &

wait
