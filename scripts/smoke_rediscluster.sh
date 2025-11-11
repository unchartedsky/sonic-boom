#!/usr/bin/env bash
set -euo pipefail

URL="http://localhost:8000/redis-cluster/api/users?page=2"

curl_h() {
	curl -s -i "$1" | tr -d '\r'
}

first=$(curl_h "$URL")
echo "$first" | grep -i '^X-Cache-Status:' || true

second=$(curl_h "$URL")
echo "$second" | grep -i '^X-Cache-Status:' || true

status1=$(echo "$first" | awk -F': ' '/^X-Cache-Status:/ {print $2}' | tr -d '[:space:]')
status2=$(echo "$second" | awk -F': ' '/^X-Cache-Status:/ {print $2}' | tr -d '[:space:]')

echo "First:  ${status1:-<none>}"
echo "Second: ${status2:-<none>}"

case "${status2:-}" in
	Hit|HIT|hit)
		echo "OK: Redis Cluster cache hit observed"
		exit 0
		;;
	*)
		echo "WARN: Redis Cluster cache hit not observed"
		exit 1
		;;
esac
