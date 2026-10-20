# babylon-cloud-auth
An authentication architecture for distributed system running with a mock cluster of services.

# Requirements
- Docker

# Get Started

Brief summary on how to run this architecture 

## Run
To start the whole architecture run ```docker compose up -d --build``` or ```start.sh```

### HTTP request
At "http://localhost:80" you can check the mock service answer to your HTTP GET request ;>

## Stop
To stop the whole architecture run ```docker compose down``` or ```stop.sh```

# Architecture
The architecture is composed of multiple mock services, an auth service, API gateways, service registries and a load balancer.

In particular, we have 2 clusters, each cluster has its own network. Inside each cluster there's exactly 1 Traefik API Gateway and exactly 1 Consul Service Registry. The API gateway service endpoints are updated at runtime by consulting the Service Registry.

Exactly 2 mock services (mock-svc) exist for each cluster. They register on Consul at start-time and provide an health check every x-amount of seconds. When the process dies, they de-register from Consul.

Exactly 1 auth service (auth-svc) exists per cluster. It self-registers with Consul under the service name `auth` and exposes a `/verify` endpoint consumed by Traefik's forward-auth middleware. Routing rules live declaratively in `traefik/dynamic.yml`: `mock-router` (protected by `auth-middleware`) and `auth-service-router` (host `auth.internal.corp`).

On top of the 2 clusters, we have exactly 1 NGINX "edge" load balancer.

## Endpoints
- mock-svc http://localhost & http://localhost/doOperation
- auth-svc http://localhost/health & http://localhost/verify (with `Host: auth.internal.corp`)
- Traefik API-Gateway Cluster 1 http://localhost:8081
- Traefik API-Gateway Cluster 2 http://localhost:8082
- Consul Service Registry Cluster 1 http://localhost:8501
- Consul Service Registry Cluster 2 http://localhost:8502

## Communcations flow
The web request reaches the edge loadbalancer. It chooses a cluster to serve said request via round robin policy.

Once the cluster is chosen, the request gets passed to the Cluster's API Gateway. At this point the gateway chooses which mock-svc instance is going to actually serve the answer. The policy here is once again round-robin.

So 4 requests in a row would land on mock-svc-1-1 (cluster 1, instance 1), then mock-svc-2-1 (cluster 2, instance 1), then mock-svc-1-2 and finally mock-svc-2-2. 

# Authentication
Requests to the mock service pass through Traefik's `auth-middleware`, which calls the auth service `/verify` endpoint. That endpoint is currently a stub (always returns 200 with `X-User-ID: anonymous`); real token validation (access/refresh JWT, signed by the auth service private key) is still to be implemented.


HOME Realm Discovery