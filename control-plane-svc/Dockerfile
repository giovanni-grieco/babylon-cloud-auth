FROM golang:1.27 AS builder

WORKDIR /src/control-plane-svc

# Standard library only, so there's no go.sum and nothing to download.
COPY control-plane-svc/go.mod ./
RUN go mod download


COPY control-plane-svc ./

#RUN go test ./... && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /control-plane-svc .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /control-plane-svc .

FROM alpine:latest

RUN apk add --no-cache tzdata

COPY --from=builder /control-plane-svc /control-plane-svc

USER nobody

CMD ["/control-plane-svc"]
