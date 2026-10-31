#!/bin/sh
set -e

cd "$(dirname "$0")"

# Create local secrets on the first run. Once Keycloak's database exists, its
# password is fixed, so reset with `docker compose down -v` if .env is deleted.
if [ ! -f .env ]; then
    umask 077
    {
        echo "KC_DB_PASSWORD=$(openssl rand -hex 24)"
        echo "KC_ADMIN_PASSWORD=$(openssl rand -hex 12)"
        echo "OIDC_CLIENT_SECRET=$(openssl rand -hex 32)"
        echo "OAUTH2_PROXY_COOKIE_SECRET=$(openssl rand -hex 16)"
    } > .env
    echo "Created .env; the Keycloak admin password is in there."
fi

docker compose up -d --build --remove-orphans
