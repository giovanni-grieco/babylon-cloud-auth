#!/bin/sh
# Stop everything and delete all Keycloak data, so the next start.sh imports the
# realm files again. Needs root: the database folders belong to the container users.
set -e

cd "$(dirname "$0")"

echo "Stopping services"
./stop.sh

echo "Deleting database files"
rm -rf keycloak-db-data keycloak-dc-1-db-data keycloak-dc-2-db-data control-plane-db-data

echo "Deleting .env file"
rm .env
