FROM ubuntu:25.10

RUN apt-get update -y && apt-get install -y wget unzip build-essential

RUN wget https://www.csie.ntu.edu.tw/~cjlin/libsvmtools/multicore-liblinear/liblinear-multicore-2.49.zip

RUN unzip liblinear-multicore-2.49.zip && rm liblinear-multicore-2.49.zip

WORKDIR /liblinear-multicore-2.49

RUN make train

WORKDIR /

RUN mv /liblinear-multicore-2.49/train liblinear-multicore-train
