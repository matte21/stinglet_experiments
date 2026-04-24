#!/bin/bash

set -o errexit

m4 -DHOST_PATH=`pwd`/pods-volumes-files pods/liblinear.yaml.m4 > pods/liblinear.yaml

m4 -DHOST_PATH=`pwd`/pods-volumes-files pods/pagerank-4-cpus.yaml.m4 > pods/pagerank-4-cpus.yaml
m4 -DHOST_PATH=`pwd`/pods-volumes-files pods/pagerank-8-cpus.yaml.m4 > pods/pagerank-8-cpus.yaml
m4 -DHOST_PATH=`pwd`/pods-volumes-files pods/pagerank-12-cpus.yaml.m4 > pods/pagerank-12-cpus.yaml

m4 -DHOST_PATH=`pwd`/pods-volumes-files pods/pagerank-4-cpus-fm.yaml.m4 > pods/pagerank-4-cpus-fm.yaml
m4 -DHOST_PATH=`pwd`/pods-volumes-files pods/pagerank-8-cpus-fm.yaml.m4 > pods/pagerank-8-cpus-fm.yaml
m4 -DHOST_PATH=`pwd`/pods-volumes-files pods/pagerank-12-cpus-fm.yaml.m4 > pods/pagerank-12-cpus-fm.yaml
