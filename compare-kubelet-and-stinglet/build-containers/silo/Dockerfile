FROM ubuntu:22.04 AS build

RUN apt-get update -y && apt-get install -y build-essential git libjemalloc2 libjemalloc-dev libnuma-dev libdb-dev libdb++-dev zlib1g-dev libaio-dev libssl-dev

RUN git clone https://github.com/cosmoss-jigu/memtis /memtis

WORKDIR /memtis/memtis-userspace/bench_dir/silo

# For stability/reproducibility.
RUN git checkout 2778c0c5593250426aec642c46ad7cb1bbdbf18a

RUN MODE=perf make -j dbtest

FROM ubuntu:22.04

RUN apt-get update -y && apt-get install -y libjemalloc2 libnuma1 libdb5.3++

COPY --from=build /memtis/memtis-userspace/bench_dir/silo/out-perf.masstree/benchmarks/dbtest /
COPY --from=build /memtis/memtis-userspace/bench_dir/silo/third-party/lz4/liblz4.so /usr/lib/x86_64-linux-gnu/
