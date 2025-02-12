#!/bin/bash
set -e
set -x

# curl --verbose https://reqres.in/api/users?page=2
curl --verbose http://localhost:8000/ex1/api/users?page=2
