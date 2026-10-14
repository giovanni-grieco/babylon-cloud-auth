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
To stop the whole architecture run "docker compose down" or "./stop.sh"

# Architecture
The architecture is composed of multiple mock services, API gateways, service registries and a load balancer.

In particular, we have 2 clusters, each cluster has its own network. Inside each cluster there's exactly 1 Traefik API Gateway and exactly 1 Consul Service Registry. The API gateway service endpoints are updated at runtime by consulting the Service Registry.

Exactly 2 mock services (mock-svc) exist for each cluster. They register on Consul at start-time and provide an health check every x-amount of seconds. When the process dies, they de-register from Consul.

On top of the 2 clusters, we have exactly 1 NGINX "edge" load balancer.

## Communcations flow
The web request reaches the edge loadbalancer. It chooses a cluster to serve said request via round robin policy.

Once the cluster is chosen, the request gets passed to the Cluster's API Gateway. At this point the gateway chooses which mock-svc instance is going to actually serve the answer. The policy here is once again round-robin.

So 4 requests in a row would land on mock-svc-1-1 (cluster 1, instance 1), then mock-svc-2-1 (cluster 2, instance 1), then mock-svc-1-2 and finally mock-svc-2-2. 

# To be implemented
So far, no authentication flow has been implemented nor any authentication systems.
