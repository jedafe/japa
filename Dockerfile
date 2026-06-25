# Dockerfile — multi-stage build for the JapaConnect Go API.
#
# Stage 1 builds a static binary from the Go source. Stage 2 copies just
# the binary into a minimal Alpine image so the final image is small and
# has a tiny attack surface.

# Stage 1: build the binary
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Cache dependencies: copy go.mod and go.sum first, download, then copy
# the rest of the source. This leverages Docker's layer cache so dependency
# downloads are skipped when only application code changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build a statically-linked binary with CGO disabled so it runs on any
# Alpine image without needing gcc or libc-dev.
RUN CGO_ENABLED=0 GOOS=linux go build -o /japa-api ./cmd/api/

# Stage 2: minimal runtime image
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /japa-api /app/japa-api

EXPOSE 8080

ENTRYPOINT ["/app/japa-api"]
