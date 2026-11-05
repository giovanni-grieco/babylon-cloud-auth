FROM maven:3.10-eclipse-temurin-21-alpine AS builder

WORKDIR /plugin

COPY ./keycloak-hrd-plugin/pom.xml .
COPY ./keycloak-hrd-plugin/src ./src

RUN mvn clean package -DskipTests

FROM quay.io/keycloak/keycloak:26.7.0

COPY --from=builder /plugin/target/keycloak-hrd-plugin-1.0.jar  /opt/keycloak/providers/keycloak-hrd-plugin.jar
