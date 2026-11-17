# Postgres 17 run by Patroni: one image for every node of every Patroni
# cluster (control plane database, central Keycloak database). Patroni decides
# which node is the primary, using etcd as the shared lock, and turns the
# others into streaming replicas.
#
# What differs per cluster comes from the environment (see patroni.yml):
#   PATRONI_SCOPE    cluster name, also its key in etcd (required)
#   PATRONI_NAME, PATRONI_RESTAPI_CONNECT_ADDRESS, PATRONI_POSTGRESQL_CONNECT_ADDRESS
#   PATRONI_SUPERUSER_*, PATRONI_REPLICATION_*
#   APP_DB_USER, APP_DB_NAME, APP_DB_PASSWORD   read by post_bootstrap.sh
# A cluster that needs different settings mounts its own file over
# /etc/patroni/patroni.yml.
FROM postgres:17

RUN apt-get update \
 && apt-get install -y --no-install-recommends patroni python3-etcd \
 && rm -rf /var/lib/apt/lists/*

COPY patroni.yml /etc/patroni/patroni.yml
COPY --chmod=755 post_bootstrap.sh /usr/local/bin/post_bootstrap.sh

USER postgres

# Patroni starts and stops Postgres itself, so skip the image's entrypoint.
ENTRYPOINT ["patroni"]
CMD ["/etc/patroni/patroni.yml"]
