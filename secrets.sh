# Sourced by start.sh and stop.sh. compose.yml requires every secret below,
# so even `docker compose down` fails until .env has them all.

# Add a random secret to .env unless it's already there, so an existing .env
# keeps its values. A Keycloak database and admin account are fixed once created.
add_secret() {
    grep -q "^$1=" .env || echo "$1=$(openssl rand -hex "$2")" >> .env
}

touch .env

add_secret KC_DB_PASSWORD 24
add_secret KC_ADMIN_USERNAME 10
add_secret KC_ADMIN_PASSWORD 12
add_secret OIDC_CLIENT_SECRET 32
add_secret OAUTH2_PROXY_COOKIE_SECRET 16
add_secret CENTRAL_CONTROL_PLANE_KEYCLOAK_CLIENT_SECRET 32

add_secret DC1_DB_PASSWORD 24
add_secret DC1_ADMIN_USERNAME 10
add_secret DC1_ADMIN_PASSWORD 12
add_secret DC1_BROKER_SECRET 32
add_secret DC1_CONTROL_PLANE_KEYCLOAK_CLIENT_SECRET 32

add_secret DC2_DB_PASSWORD 24
add_secret DC2_ADMIN_USERNAME 10
add_secret DC2_ADMIN_PASSWORD 12
add_secret DC2_BROKER_SECRET 32
add_secret DC2_CONTROL_PLANE_KEYCLOAK_CLIENT_SECRET 32

add_secret CONTROL_PLANE_SECRET 32
add_secret CONTROL_PLANE_DB_PASSWORD 32