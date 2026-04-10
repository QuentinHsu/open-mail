FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /open-mail ./cmd/open-mail

FROM alpine:3.21
WORKDIR /app
RUN adduser -D appuser
COPY --from=builder /open-mail /usr/local/bin/open-mail
COPY .env.example /app/.env.example
RUN mkdir -p /app/.data && chown -R appuser:appuser /app
USER appuser
EXPOSE 3000
CMD ["open-mail"]
