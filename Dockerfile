# BUILD PHASE
FROM golang:alpine AS builder

WORKDIR /build
COPY . .

# Download dependencies
RUN go mod download

# Build the executable with Linux flags
RUN GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /app/isley

# IMAGE PHASE
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/isley /app/isley

# Add database directory
RUN mkdir data && touch data/isley.db

EXPOSE 8080

ENTRYPOINT ["/app/isley"]
