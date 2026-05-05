#!/bin/bash

set -o errexit

sudo apt-get update -y && sudo apt-get install -y libsystemd-dev

echo building trace player binary...
go mod tidy
go build -race -o play_trace

chmod +x play_trace
cp play_trace ..
