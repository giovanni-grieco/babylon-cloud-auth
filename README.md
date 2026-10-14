# babylon-cloud-auth
An authentication architecture for distributed system running with a mock cluster of services.

## Run and access the services

Start the stack with:

```sh
docker compose up -d
```

The public entrypoint is `http://localhost`. The protected mock service is available at:

```text
http://localhost/api/mock/get
```

Opening that URL shows the OAuth2 sign-in page, which sends you to Keycloak. Use the imported demo account (`demo` / `demo`) or
create another user in the Keycloak admin console at `http://localhost:8443` (admin / admin).
The Keycloak realm used by the gateway is `babylon`.

The gateway dashboard is available at `http://localhost:8090`. A request to `/` returns 404
because no application route is configured for the root path; use `/api/mock/...` or add another
service to [traefik/dynamic.yml](./traefik/dynamic.yml).
