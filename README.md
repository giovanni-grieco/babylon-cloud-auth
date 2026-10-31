# babylon-cloud-auth
An authentication architecture for distributed system running with a mock cluster of services.

# Requirements
- Docker
- These lines in `/etc/hosts`:
  ```text
  127.0.0.1   babylon.test auth.babylon.test
  ```

# Get Started

## Run
Run ```./start.sh```. On the first run it generates the local secrets in `.env`, which `docker compose` needs.

Then open http://babylon.test. Keycloak takes a little while to start the first time.

Test users (username / password):
- `alice` / `alice`, in group `service-a`
- `bob` / `bob`, in group `service-b`

You can also register a new user from the login page.

## Stop
To stop the whole architecture run ```docker compose down``` or ```stop.sh```.

To also delete Keycloak's database, and re-import `keycloak/import/babylon-realm.json` on the next start, run ```docker compose down -v```.

# Architecture
The architecture is composed of multiple mock services, an identity provider, API gateways, service registries and a load balancer.

In particular, we have 2 clusters, each cluster has its own network. Inside each cluster there's exactly 1 Traefik API Gateway and exactly 1 Consul Service Registry. The API gateway service endpoints are updated at runtime by consulting the Service Registry.

Exactly 2 mock services (mock-svc) exist for each cluster. They register on Consul at start-time and provide an health check every x-amount of seconds. When the process dies, they de-register from Consul.

On top of the 2 clusters, we have exactly 1 NGINX "edge" load balancer, 1 Keycloak instance with its Postgres database, and 1 oauth2-proxy instance shared by both gateways.

## Endpoints
- mock-svc http://babylon.test (public) & http://babylon.test/doOperation (login required)
- Keycloak http://auth.babylon.test; the admin console user is `admin`, with the password in `.env`
- Traefik API-Gateway Cluster 1 http://localhost:8081
- Traefik API-Gateway Cluster 2 http://localhost:8082
- Consul Service Registry Cluster 1 http://localhost:8501
- Consul Service Registry Cluster 2 http://localhost:8502

## Communications flow
The web request reaches the edge load balancer, which routes on the `Host` header. `auth.babylon.test` goes to Keycloak, and everything else goes to a cluster's API Gateway, chosen by hashing the client address.

The gateway then chooses which mock-svc instance serves the answer, using round robin.

# Authentication
Keycloak is the identity provider, configured by the `babylon` realm in `keycloak/import/babylon-realm.json`. This file is only imported when the realm doesn't exist yet. Customer organisations use Keycloak Organizations, and each can link its own SAML or OIDC identity provider.

Protected routes use Traefik's `auth-middleware`, which calls oauth2-proxy on every request:
- **Valid session:** oauth2-proxy answers 202, and Traefik passes `X-Auth-Request-User`, `X-Auth-Request-Preferred-Username`, `X-Auth-Request-Email` and `X-Auth-Request-Groups` to the service. Services must only be reachable through the gateway, because they trust these headers.
- **No session:** oauth2-proxy redirects the browser to Keycloak. After login it stores the tokens in Redis (`state-kv`) and sets its `_oauth2_proxy` cookie.

Access tokens last 5 minutes, and oauth2-proxy refreshes them after 4 minutes on the next request. Keycloak's SSO session ends after 30 idle minutes or 10 hours.

The logout link goes to `/oauth2/sign_out`, which ends the oauth2-proxy session, then redirects to Keycloak's logout endpoint to end the SSO session.
