FROM ubuntu:22.04

RUN apt-get update -y && apt-get install -y build-essential git m4

RUN git clone https://github.com/bamos/parsec-benchmark

WORKDIR /parsec-benchmark

RUN git checkout 2e492f771c6749577fe4e7610d516a9e7f8ad580

RUN bin/parsecmgmt -a build -p blackscholes

COPY input_native.tar pkgs/apps/blackscholes/inputs/
