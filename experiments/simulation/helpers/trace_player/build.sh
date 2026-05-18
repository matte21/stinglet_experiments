#!/bin/bash

set -o errexit

if ! dpkg -l libsystemd-dev 2>/dev/null | grep -q "^ii"; then
    sudo apt-get update -y && sudo apt-get install -y libsystemd-dev
fi

go mod tidy

echo building trace player...
go build -race -o play_trace

echo building trace analyzer...
go build -race -o analyze_trace ./cmd/trace_analyzer/

chmod +x play_trace
mv play_trace ..

chmod +x analyze_trace
mv analyze_trace ..

