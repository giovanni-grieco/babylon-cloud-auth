#!/bin/sh
# Run by Patroni once, on the node that creates the cluster. $1 is a
# superuser connection string. Replicas copy the result through replication.
# Creates the application's role and database from APP_DB_USER, APP_DB_NAME
# and APP_DB_PASSWORD.
set -eu

: "${APP_DB_USER:?APP_DB_USER is not set}"
: "${APP_DB_NAME:?APP_DB_NAME is not set}"
: "${APP_DB_PASSWORD:?APP_DB_PASSWORD is not set}"

psql "$1" -v ON_ERROR_STOP=1 \
  -v app_user="$APP_DB_USER" \
  -v app_password="$APP_DB_PASSWORD" \
  -v app_db="$APP_DB_NAME" <<'SQL'
CREATE ROLE :"app_user" LOGIN PASSWORD :'app_password';
CREATE DATABASE :"app_db" OWNER :"app_user";
SQL
