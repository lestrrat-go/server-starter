#!/bin/bash

set -euo pipefail

RELENG_DIR=$(cd "$(dirname "$0")"; pwd -P)
docker build -t server_starter-docker "$RELENG_DIR"