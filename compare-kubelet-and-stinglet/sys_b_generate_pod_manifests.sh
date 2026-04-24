#!/bin/bash

set -o errexit

m4 -DID=1 -DHOST_PATH=`pwd`/pods-volumes-files sys-b-pods/liblinear.yaml.m4 > sys-b-pods/liblinear-1.yaml
m4 -DID=2 -DHOST_PATH=`pwd`/pods-volumes-files sys-b-pods/liblinear.yaml.m4 > sys-b-pods/liblinear-2.yaml
m4 -DID=3 -DHOST_PATH=`pwd`/pods-volumes-files sys-b-pods/liblinear.yaml.m4 > sys-b-pods/liblinear-3.yaml
m4 -DID=4 -DHOST_PATH=`pwd`/pods-volumes-files sys-b-pods/liblinear.yaml.m4 > sys-b-pods/liblinear-4.yaml

m4 -DHOST_PATH=`pwd`/pods-volumes-files sys-b-pods/pagerank.yaml.m4 > sys-b-pods/pagerank.yaml
