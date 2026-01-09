# BUILD PHASE
FROM golang:alpine AS builder
WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH

# Build the executable with OS specific flags
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -o /app/isley

# IMAGE PHASE
FROM alpine:latest
WORKDIR /app

# Install tzdata for runtime configuration & ffmpeg for video processing
RUN apk add --no-cache tzdata ffmpeg

# Copy the built application
COPY --from=builder /app/isley /app/isley

# Add database directory
RUN mkdir -p /app/data
VOLUME ["/app/data"]

# Set default timezone as UTC but allow override with ENV variable
ENV TZ=UTC
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

EXPOSE 8080

ENTRYPOINT ["/app/isley"]
