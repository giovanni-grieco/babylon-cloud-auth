# babylon-cloud-auth
An authentication architecture for distributed system running with a mock cluster of services.

# Requirements
- Docker
- The Babylon line from `hosts.example` in `/etc/hosts`

# Get Started

## Run
Run ```./start.sh```. It adds any missing local secrets to `.env`, which `docker compose` needs, and keeps the existing ones.

Then open http://babylon.test. Keycloak takes a little while to start the first time.

Test users (email / password):
- `alice@dc1.test` / `alice`, in `dc-1`, group `service-a`
- `bob@dc2.test` / `bob`, in `dc-2`, group `service-b`

Self-registration is off. Users are provisioned in their DC's Keycloak.

## Stop
To stop the whole architecture run ```docker compose down``` or ```stop.sh```.

Keycloak only imports a realm file when that realm doesn't exist yet. To apply changes to the realm files, stop the stack and delete the database folders, which also deletes all Keycloak data. They're owned by the container users, so this needs `sudo`:
```sh
sudo rm -rf keycloak-db-data keycloak-dc-1-db-data keycloak-dc-2-db-data
```

# Architecture
The architecture is composed of multiple mock services, identity providers, API gateways, service registries and a load balancer.

In particular, we have 2 clusters, each cluster has its own network. Inside each cluster there's exactly 1 Traefik API Gateway, exactly 1 oauth2-proxy and exactly 1 Consul Service Registry. Each gateway reaches its own oauth2-proxy through the `oauth2-proxy` alias on its cluster network. Both instances share the same cookie secret and Redis, so a session works on either cluster. The API gateway service endpoints are updated at runtime by consulting the Service Registry.

Exactly 2 mock services (mock-svc) exist for each cluster. They register on Consul at start-time and provide an health check every x-amount of seconds. When the process dies, they de-register from Consul.

On top of the 2 clusters, we have exactly 1 NGINX "edge" load balancer, 1 central Keycloak broker, and 1 Keycloak per DC. Each Keycloak has its own Postgres database, on a private network that only that Keycloak can reach.

## Endpoints
- mock-svc http://babylon.test (public) & http://babylon.test/doOperation (login required)
- Central Keycloak http://auth.babylon.test; the admin console credentials are `KC_ADMIN_USERNAME` and `KC_ADMIN_PASSWORD` in `.env`
- dc-1 Keycloak http://auth.dc-1.babylon.test; the admin console credentials are `DC1_ADMIN_USERNAME` and `DC1_ADMIN_PASSWORD`
- dc-2 Keycloak http://auth.dc-2.babylon.test; the admin console credentials are `DC2_ADMIN_USERNAME` and `DC2_ADMIN_PASSWORD`
- Traefik API-Gateway Cluster 1 http://localhost:8081
- Traefik API-Gateway Cluster 2 http://localhost:8082
- Consul Service Registry Cluster 1 http://localhost:8501
- Consul Service Registry Cluster 2 http://localhost:8502

## Communications flow
The web request reaches the edge load balancer, which routes on the `Host` header. The `auth.*` names go to their Keycloak, and everything else goes to a cluster's API Gateway, chosen by hashing the client address.

The gateway then chooses which mock-svc instance serves the answer, using round robin.

# Authentication
The central Keycloak (`keycloak/central`) is the single login point. It holds no passwords or user profiles, and brokers each login to the Keycloak that owns the user (`keycloak/dc-1`, `keycloak/dc-2`).

## Login
1. A protected route uses Traefik's `auth-middleware`, which calls oauth2-proxy. With no session, oauth2-proxy redirects the browser to the central Keycloak.
2. The user enters their email. The [home-idp-discovery](https://github.com/sventorben/keycloak-home-idp-discovery) plugin matches the domain against each identity provider's `home.idp.discovery.domains` setting and redirects to that DC.
3. The DC's Keycloak authenticates the user, and returns only an opaque ID and their groups to the central Keycloak.
4. The central Keycloak creates or updates its user `<dc>.<dc user id>` and issues its own tokens.
5. oauth2-proxy stores the tokens in Redis (`state-kv`) and sets its `_oauth2_proxy` cookie.

On later requests, oauth2-proxy answers 202 and Traefik passes `X-Auth-Request-User`, `X-Auth-Request-Preferred-Username` and `X-Auth-Request-Groups` to the service. Services must only be reachable through the gateway, because they trust these headers. `X-Auth-Request-Email` holds the user ID rather than an address, because the central Keycloak has no emails.

Access tokens last 5 minutes, and oauth2-proxy refreshes them after 4 minutes on the next request. Keycloak's SSO session ends after 30 idle minutes or 10 hours.

## Logout
The logout link goes to `/oauth2/sign_out`, which ends the oauth2-proxy session, then redirects to the central Keycloak's logout endpoint. That is configured to also end the session at the user's DC.

## Adding a customer
1. Pick the DC from the contract.
2. Create the Organization in that DC's Keycloak, and link the customer's identity provider if it has one.
3. Add the domain to that DC's `home.idp.discovery.domains` setting in the central Keycloak, with domains separated by `##`.

An email domain that isn't configured ends on an error page, because the central Keycloak has no local users to fall back on.

## Upgrading Keycloak
`keycloak/central/Dockerfile` pins a plugin release that must match the Keycloak version. Upgrade all three Keycloak versions and the plugin together.
