FROM ubuntu:25.10

RUN apt-get update -y && apt-get install -y git build-essential

RUN git clone https://github.com/sbeamer/gapbs.git

WORKDIR /gapbs

RUN make pr

WORKDIR /

RUN mv /gapbs/pr pagerank
