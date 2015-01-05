#!/bin/bash

DIR=/work/src/github.com/lestrrat/go-server-starter

pushd $DIR
go get github.com/jessevdk/go-flags
goxc \
    -n start_server \
    -tasks "xc archive" \
    -bc "linux windows darwin" \
    -wd $DIR \
    -resources-include "README*,Changes" \
    -d /work/artifacts