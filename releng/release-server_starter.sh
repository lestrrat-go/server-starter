#!/bin/bash

set -e

if [ -z "$GITHUB_TOKEN" ]; then
    echo "GITHUB_TOKEN environment variable must be set"
    exit 1
fi

if [ -z "$GITHUB_USERNAME" ]; then
    echo "GITHUB_USERNAME environment variable must be set"
    exit 1
fi

if [ -z "$SS_VERSION" ]; then
    echo "SS_VERSION environment variable must be set"
    exit 1
fi

# Change directory to the project because that makes
# things much easier
cd /work/src/github.com/lestrrat/go-server-starter

/build-server_starter.sh
ghr --debug -p 1 --replace -u "$GITHUB_USERNAME" $SS_VERSION /work/artifacts/snapshot