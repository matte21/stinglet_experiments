FROM ubuntu:25.10

RUN apt-get update -y && apt-get install -y build-essential libomp-dev git

RUN git clone https://github.com/ANL-CESAR/XSBench /XSBench

WORKDIR /XSBench/openmp-threading

RUN git checkout v20

RUN make

WORKDIR /

RUN mv /XSBench/openmp-threading/XSBench xsbench
