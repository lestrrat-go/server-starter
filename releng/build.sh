#!/bin/bash

set -e

SSDIR=$(cd $(dirname $0)/..; pwd -P)
id=$(echo $(date) $$| shasum | awk '{print $1}')
docker run --rm \
    --name server_starter-build-$id \
    -v $SSDIR:/work/src/github.com/lestrrat/go-server-starter/ \
    -e RESULTSDIR=/work/artifacts \
    server_starter-docker \
    ./build-server_starter.sh