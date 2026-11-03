FROM golang:latest AS builder

WORKDIR /src

COPY common/go.mod ./common/

COPY control-plane-svc/go.mod control-plane-svc/go.sum ./control-plane-svc/

WORKDIR /src/control-plane-svc

RUN go mod download

WORKDIR /src

COPY common ./common

COPY control-plane-svc ./control-plane-svc

WORKDIR /src/control-plane-svc

RUN CGO_ENABLED=0 GOOS=linux go build -o /control-plane-svc

FROM alpine:latest 

COPY --from=builder /control-plane-svc /control-plane-svc

CMD ["/control-plane-svc"]