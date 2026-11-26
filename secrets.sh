# Sourced by start.sh and stop.sh. compose.yml requires every secret below,
# so even `docker compose down` fails until .env has them all.

# Add a random secret to .env unless it's already there, so an existing .env
# keeps its values. A Keycloak database and admin account are fixed once created.
add_secret() {
    grep -q "^$1=" .env || echo "$1=$(openssl rand -hex "$2")" >> .env
}

touch .env

add_secret CENTRAL_KC_DB_PASSWORD 24
add_secret CENTRAL_KC_DB_SUPERUSER_PASSWORD 32
add_secret CENTRAL_KC_DB_REPLICATION_PASSWORD 32
add_secret CENTRAL_KC_ADMIN_USERNAME 10
add_secret CENTRAL_KC_ADMIN_PASSWORD 12
add_secret OAUTH2_PROXY_OIDC_CLIENT_SECRET 32
add_secret OAUTH2_PROXY_COOKIE_SECRET 16
add_secret CONTROL_PLANE_CENTRAL_KEYCLOAK_CLIENT_SECRET 32

add_secret DC1_KEYCLOAK_DB_PASSWORD 24
add_secret DC1_KEYCLOAK_ADMIN_USERNAME 10
add_secret DC1_KEYCLOAK_ADMIN_PASSWORD 12
add_secret CENTRAL_KEYCLOAK_DC1_BROKER_SECRET 32
add_secret DC1_CONTROL_PLANE_KEYCLOAK_CLIENT_SECRET 32

add_secret DC2_KEYCLOAK_DB_PASSWORD 24
add_secret DC2_KEYCLOAK_ADMIN_USERNAME 10
add_secret DC2_KEYCLOAK_ADMIN_PASSWORD 12
add_secret CENTRAL_KEYCLOAK_DC2_BROKER_SECRET 32
add_secret DC2_CONTROL_PLANE_KEYCLOAK_CLIENT_SECRET 32

add_secret CONTROL_PLANE_CLIENT_SECRET 32
add_secret CONTROL_PLANE_DB_PASSWORD 32
add_secret CONTROL_PLANE_DB_SUPERUSER_PASSWORD 32
add_secret CONTROL_PLANE_DB_REPLICATION_PASSWORD 32

# Operator login for the control plane console (its own Keycloak)
add_secret CP_KC_DB_PASSWORD 24
add_secret CP_KC_ADMIN_USERNAME 10
add_secret CP_KC_ADMIN_PASSWORD 12
add_secret CP_CONSOLE_OIDC_CLIENT_SECRET 32
add_secret CP_CONSOLE_COOKIE_SECRET 16
add_secret CP_OPERATOR_PASSWORD 12

add_secret BLOG_DB_USER 12
add_secret BLOG_DB_PASSWORD 24
