FROM ubuntu:22.04

RUN apt-get update -y && apt-get install -y build-essential git

RUN git clone https://github.com/graph500/graph500

WORKDIR /graph500

RUN git checkout graph500-2.1.4 && cp make-incs/make.inc-gcc make.inc

RUN sed -i 's/^CPPFLAGS/#CPPFLAGS/' make.inc && \
    sed -i 's/^CFLAGS = -g -std=c99$/#CFLAGS = -g -std=c99/' make.inc && \
    sed -i 's/^#CFLAGS = -g -std=c99 -O3/CFLAGS = -g -std=c99 -O3/' make.inc && \
    sed -i 's/^int NBFS = NBFS_max;$/int NBFS = 14;/' options.c

RUN make omp-csr/omp-csr && mv omp-csr/omp-csr /omp-csr
