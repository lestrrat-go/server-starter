#!/bin/bash

set -e

SSDIR=$(cd $(dirname $0)/..; pwd -P)

if [ -z "$SS_VERSION" ]; then
    echo "SS_VERSION must be specified"
    exit 1
fi

if [ -z "$GITHUB_TOKEN_FILE" ]; then
    GITHUB_TOKEN_FILE=github_token
fi

if [ ! -e "$GITHUB_TOKEN_FILE" ]; then
    echo "GITHUB_TOKEN_FILE does not exist"
    exit 1
fi

docker run --rm \
    -v $SSDIR:/work/src/github.com/lestrrat/go-server-starter/ \
    -e SS_VERSION=$SS_VERSION \
    -e GITHUB_USERNAME=lestrrat \
    -e GITHUB_TOKEN=`cat $GITHUB_TOKEN_FILE` \
    server_starter-docker \
    /release-server_starter.sh

