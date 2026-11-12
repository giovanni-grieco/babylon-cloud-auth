#!/bin/sh
# Run by Patroni once, on the node that creates the cluster. $1 is a
# superuser connection string. Replicas copy the result through replication.
set -eu

psql "$1" -v ON_ERROR_STOP=1 \
  -v app_user="$CONTROL_PLANE_DB_USER" \
  -v app_password="$CONTROL_PLANE_DB_PASSWORD" \
  -v app_db="$CONTROL_PLANE_DB_NAME" <<'SQL'
CREATE ROLE :"app_user" LOGIN PASSWORD :'app_password';
CREATE DATABASE :"app_db" OWNER :"app_user";
SQL
