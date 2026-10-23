FROM golang:latest AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /central-auth-svc

FROM alpine:latest
COPY --from=builder /central-auth-svc /central-auth-svc
CMD ["/central-auth-svc"]
