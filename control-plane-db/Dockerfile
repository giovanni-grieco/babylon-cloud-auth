# Postgres 17 run by Patroni: one image for every node of the control plane
# database cluster. Patroni decides which node is the primary, using etcd as
# the shared lock, and turns the others into streaming replicas.
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
