FROM maven:3.10-eclipse-temurin-21-alpine AS builder

WORKDIR /blog

# Copy only the pom first so dependency resolution is cached as its own layer
# and is re-run only when pom.xml changes, not on every source edit.
COPY ./pom.xml .
RUN --mount=type=cache,target=/root/.m2 \
    mvn -B dependency:go-offline

COPY ./src ./src

# The cache mount keeps ~/.m2 across builds, so anything go-offline missed
# (e.g. plugins resolved lazily during package) is downloaded only once.
RUN --mount=type=cache,target=/root/.m2 \
    mvn -B clean package -DskipTests


FROM eclipse-temurin:21

WORKDIR /app

COPY --from=builder /blog/target/blog-0.0.1-SNAPSHOT.jar  /app/blog-svc.jar

ENTRYPOINT [ "java", "-jar", "blog-svc.jar" ]

