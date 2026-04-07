FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o ipinfo .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/ipinfo .
COPY --from=builder /app/templates ./templates

# Create the cache directory
RUN mkdir -p /var/cache/ipinfo && chmod 777 /var/cache/ipinfo

EXPOSE 8080

CMD ["./ipinfo"]
