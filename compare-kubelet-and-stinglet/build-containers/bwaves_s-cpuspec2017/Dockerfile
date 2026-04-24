FROM ubuntu:25.10

RUN apt-get update && apt-get install -y \
    libgfortran5 \
    libgomp1 \
    libquadmath0 \
    && rm -rf /var/lib/apt/lists/*

COPY bwaves_1.in bwaves_1.in

COPY speed_bwaves_base.mytest-m64 speed_bwaves_base.mytest-m64
