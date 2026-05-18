#!/bin/bash

set -o errexit

sudo apt-get update -y && sudo apt-get install -y libsystemd-dev

go mod tidy

echo building trace player...
go build -race -o play_trace

echo building trace analyzer...
go build -race -o analyze_trace cmd/trace_analyzer/

chmod +x play_trace
mv play_trace ..

chmod +x analyze_trace
mv analyze_trace ..

