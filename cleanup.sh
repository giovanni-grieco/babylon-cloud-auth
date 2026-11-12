#!/bin/sh
# Stop everything and delete all Keycloak data, so the next start.sh imports the
# realm files again. Also removes the control plane database volumes (Patroni
# nodes and etcd), so the cluster is bootstrapped again.
# Needs root: the database folders belong to the container users.
set -e

cd "$(dirname "$0")"

echo "Stopping services"
. ./secrets.sh
docker compose down --volumes --remove-orphans

echo "Deleting database files"
rm -rf keycloak-db-data keycloak-dc-1-db-data keycloak-dc-2-db-data control-plane-db-data
