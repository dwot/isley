# BUILD PHASE
FROM golang:alpine AS builder

WORKDIR $GOPATH/src/isley
COPY . .
RUN go mod download

# Build
RUN GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /go/bin/isley

# IMAGE PHASE
FROM alpine:latest

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/bin/isley /app/isley
COPY web/templates ./web/templates
COPY web/static ./web/static
COPY migrations ./migrations
# make the data dir and initialize the isley.db file
RUN mkdir data && touch data/isley.db

EXPOSE 8666

ENTRYPOINT [ "/app/isley" ]