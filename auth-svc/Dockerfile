FROM golang:latest AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /auth-svc

FROM alpine:latest 

COPY --from=builder /auth-svc /auth-svc

CMD ["/auth-svc"]