FROM ubuntu:25.10

RUN apt-get update -y && apt-get install -y build-essential libomp-dev git libnuma-dev

RUN git clone https://github.com/mitosis-project/vmitosis-workloads /vmitosis-workloads

WORKDIR /vmitosis-workloads

# Pick a specific commit for reproducibility.
RUN git checkout 25b5ede

# Ensure the code has 157 million key-val pairs (making up for a ~15.2 GiB working set),
# same as the Memtis paper.
RUN sed -i 's/^#define NELEMENTS.*$/#define NELEMENTS 157000000UL/' btree/btree.c

# Ensure the code performs 1 billion lookups (Memtis paper uses 8 billions, we use only 1 to make
# the experiments run faster).
RUN sed -i 's/^#define NLOOKUP.*$/#define NLOOKUP 1000000000UL/' btree/btree.c

RUN make btree

WORKDIR /

RUN mv /vmitosis-workloads/bin/bench_btree_mt bench_btree_mt
