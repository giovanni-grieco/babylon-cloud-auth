#!/bin/sh
set -e

cd "$(dirname "$0")"

. ./secrets.sh

docker compose up -d --build --remove-orphans
