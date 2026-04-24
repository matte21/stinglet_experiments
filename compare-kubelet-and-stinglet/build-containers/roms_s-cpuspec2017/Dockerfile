FROM ubuntu:25.10

RUN apt-get update -y && apt-get install -y \
    libgfortran5 \
    libgomp1 \
    libquadmath0 \
    && rm -rf /var/lib/apt/lists/*

COPY varinfo.dat varinfo.dat

COPY ocean_benchmark3.in ocean_benchmark3.in

COPY sroms_base.mytest-m64 sroms_base.mytest-m64
