#!/bin/bash
set -e
set -x

if ! (type ab > /dev/null); then
  brew install ab
fi

if ! (type wait-for > /dev/null); then
  go install github.com/dnnrly/wait-for/cmd/wait-for@latest
fi

echo "Benchmarking..."

docker-compose stop || /bin/true
docker-compose rm -f || /bin/true

echo "Without a plugin..."

docker-compose up -d

wait-for "http://localhost:8000/noplugins/api/users?page=1"
echo "All services are up and running!"

ab -n 1000 -k -c 4 \
  -A "username1:password1" \
  "http://localhost:8000/noplugins/api/users?page=1"

docker-compose stop || /bin/true
docker-compose rm -f || /bin/true


echo "With a plugin..."
docker-compose up -d

wait-for "http://localhost:8000/ex1/api/users?page=2"
echo "All services are up and running!"

ab -n 1000 -k -c 4 \
  -A "username2:password1" \
  "http://localhost:8000/ex1/api/users?page=2"

docker-compose stop || /bin/true
docker-compose rm -f || /bin/true

