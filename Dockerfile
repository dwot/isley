# BUILD PHASE
FROM golang:alpine AS builder

WORKDIR /build
COPY . .

# Install tzdata for time zone configuration
RUN apk add --no-cache tzdata

# Download dependencies
RUN go mod download

# Build the executable with Linux flags
RUN GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /app/isley

# IMAGE PHASE
FROM alpine:latest

WORKDIR /app

# Install tzdata for runtime configuration
RUN apk add --no-cache tzdata

# Copy the built application
COPY --from=builder /app/isley /app/isley

# Add database directory
RUN mkdir data && touch data/isley.db

# Set default timezone as UTC but allow override with ENV variable
ENV TZ=UTC
RUN ln -sf /usr/share/zoneinfo/$TZ /etc/localtime && \
    echo $TZ > /etc/timezone

EXPOSE 8080

ENTRYPOINT ["/app/isley"]
