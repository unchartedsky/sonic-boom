#!/bin/bash
set -e
set -x

if ! (type air > /dev/null); then
  go install github.com/air-verse/air@latest
  # curl -sSfL https://goblin.run/github.com/air-verse/air | sh
fi

if ! (type docker-compose > /dev/null); then
  brew install docker-compose
fi

if [[ "${GOOS}" == "" ]]; then
  GOOS=linux
fi
if [[ "${GOARCH}" == "" ]]; then
  GOARCH="${ARCH}"
fi

mkdir -p "bin/${GOOS}-${GOARCH}"

# Run docker
docker-compose stop || true
docker-compose rm -f || true

if [[ ! -f "bin/${GOOS}-${GOARCH}/sonic-boom" ]]; then
  GOOS="${GOOS}" GOARCH="${GOARCH}" ./build.sh
fi
docker-compose up --build -d

# Hot Reload
#air -c .air.toml
air \
  --build.include_ext 'go,tpl,tmpl,html,yaml,yml' \
  --build.cmd "GOOS=${GOOS} GOARCH=${GOARCH} ./build.sh" \
  --build.bin "docker-compose exec -T kong kong reload"
