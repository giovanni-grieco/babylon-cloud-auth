# syntax=docker/dockerfile:1
FROM maven:3.10-eclipse-temurin-21-alpine AS builder

WORKDIR /plugin

# Copy only the pom first so dependency resolution is cached as its own layer
# and is re-run only when pom.xml changes, not on every source edit.
COPY ./keycloak-hrd-plugin/pom.xml .
RUN --mount=type=cache,target=/root/.m2 \
    mvn -B dependency:go-offline

COPY ./keycloak-hrd-plugin/src ./src

# The cache mount keeps ~/.m2 across builds, so anything go-offline missed
# (e.g. plugins resolved lazily during package) is downloaded only once.
RUN --mount=type=cache,target=/root/.m2 \
    mvn -B clean package -DskipTests

FROM quay.io/keycloak/keycloak:26.7.0

COPY --from=builder /plugin/target/keycloak-hrd-plugin-1.0.jar  /opt/keycloak/providers/keycloak-hrd-plugin.jar
