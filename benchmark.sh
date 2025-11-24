#!/bin/bash
set -e
set -x

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

if ! (type ab > /dev/null); then
  brew install ab
fi

if ! (type wait-for > /dev/null); then
  go install github.com/dnnrly/wait-for/cmd/wait-for@latest
fi

echo "${GREEN}Building...${NC}"
./build.sh

echo "${BLUE}Running Go Benchmarks...${NC}"
echo "In-Memory Cache Benchmarks:"
go test -bench=Benchmark_InMemory -benchmem ./internal

echo ""
echo "Marshaler Benchmarks:"
go test -bench=Benchmark_InMemory_Marshal -benchmem ./internal

echo ""
echo "Concurrency Benchmarks:"
go test -bench=Benchmark_InMemory_Concurrent -benchmem ./internal

echo ""
echo "${GREEN}HTTP Load Testing...${NC}"

docker-compose stop || /bin/true

echo "${GREEN}Without a plugin...${NC}"

docker-compose up kong -d

wait-for "http://localhost:8000/noplugins/api/users?page=1"
echo "All services are up and running!"

ab -n 1000 -k -c 4 \
  -A "username1:password1" \
  "http://localhost:8000/noplugins/api/users?page=1"

docker-compose stop || /bin/true


echo "${GREEN}With a in-memory caching plugin...${NC}"
docker-compose up kong -d

wait-for "http://localhost:8000/in-memory/api/users?page=2"
echo "All services are up and running!"

ab -n 1000 -k -c 4 \
  -A "username2:password1" \
  "http://localhost:8000/in-memory/api/users?page=2"

docker-compose stop || /bin/true


echo "${GREEN}With a Redis caching plugin...${NC}"
docker-compose up -d

wait-for "http://localhost:8000/redis/api/users?page=2"
echo "All services are up and running!"

ab -n 1000 -k -c 4 \
  -A "username2:password1" \
  "http://localhost:8000/redis/api/users?page=2"

docker-compose stop || /bin/true
