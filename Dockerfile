FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o kv-server ./src/main.go

# Final image
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/kv-server .

EXPOSE 8080

ENV PORT=8080

CMD ["./kv-server"]