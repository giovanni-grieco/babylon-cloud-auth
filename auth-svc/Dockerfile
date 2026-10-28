FROM golang:latest AS builder

WORKDIR /src

COPY common/go.mod ./common/

COPY auth-svc/go.mod auth-svc/go.sum ./auth-svc/

WORKDIR /src/auth-svc

RUN go mod download

WORKDIR /src

COPY common ./common

COPY auth-svc ./auth-svc

WORKDIR /src/auth-svc

RUN CGO_ENABLED=0 GOOS=linux go build -o /auth-svc

FROM alpine:latest 

COPY --from=builder /auth-svc /auth-svc

CMD ["/auth-svc"]