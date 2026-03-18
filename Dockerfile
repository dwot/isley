# BUILD PHASE
FROM golang:1.25.0-alpine3.21 AS builder
WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH

# Build the executable with OS specific flags
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -o /app/isley

# IMAGE PHASE
FROM alpine:3.21
WORKDIR /app

# Install tzdata for runtime configuration, ffmpeg for video processing,
# and su-exec for lightweight privilege drop in the entrypoint.
# Versions are pinned transitively via the alpine:3.21 base image.
# Log installed versions for build auditing and reproducibility.
RUN apk add --no-cache tzdata ffmpeg su-exec \
    && echo "--- Installed package versions ---" \
    && apk info -v tzdata ffmpeg su-exec

# Copy the built application and entrypoint
COPY --from=builder /app/isley /app/isley
COPY entrypoint.sh /app/entrypoint.sh

# Add database and uploads directories
RUN mkdir -p /app/data /app/uploads

# Create a non-root user to run the application
RUN addgroup -S isley && adduser -S isley -G isley \
    && chown -R isley:isley /app

VOLUME ["/app/data"]

# Set default timezone as UTC but allow override with ENV variable
ENV TZ=UTC

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

# Entrypoint runs as root to fix volume ownership, then drops to isley user
ENTRYPOINT ["/app/entrypoint.sh"]
