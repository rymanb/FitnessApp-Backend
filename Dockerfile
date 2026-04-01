# Stage 1: Build
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Download dependencies first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/api

# Stage 2: Runtime
FROM alpine:3.21

# ca-certificates required for Google AI API calls
RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/server .

EXPOSE 8080

CMD ["./server"]
