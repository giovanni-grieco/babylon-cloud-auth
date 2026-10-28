FROM golang:latest AS builder

WORKDIR /src

COPY common/go.mod ./common/

COPY central-auth-svc/go.mod central-auth-svc/go.sum ./central-auth-svc/

WORKDIR /src/central-auth-svc

RUN go mod download

WORKDIR /src

COPY common ./common

COPY central-auth-svc ./central-auth-svc

WORKDIR /src/central-auth-svc

RUN CGO_ENABLED=0 GOOS=linux go build -o /central-auth-svc

FROM alpine:latest 

COPY --from=builder /central-auth-svc /central-auth-svc

CMD ["/central-auth-svc"]