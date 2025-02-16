#!/bin/bash
set -e
set -x

if [[ "${GOOS}" == "" ]]; then
  GOOS=linux
fi
if [[ "${GOARCH}" == "" ]]; then
  GOARCH="${ARCH}"
fi

mkdir -p "bin/${GOOS}-${GOARCH}"

GOOS="${GOOS}" GOARCH="${GOARCH}" go build -o "bin/${GOOS}-${GOARCH}/sonic-boom" main.go
