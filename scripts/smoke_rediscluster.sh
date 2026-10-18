#!/usr/bin/env bash
set -euo pipefail

"$(dirname "$0")/smoke_lib.sh" "rediscluster" "/redis-cluster"
