# Dockerfile — multi-stage build for the JapaConnect Go API.
#
# Stage 1 builds a static binary from the Go source. Stage 2 copies just
# the binary into a minimal Alpine image so the final image is small and
# has a tiny attack surface.

# Stage 1: build the binary
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Install git so the build can derive the deployment version from the
# git commit count.
RUN apk add --no-cache git

# Cache dependencies: copy go.mod and go.sum first, download, then copy
# the rest of the source. This leverages Docker's layer cache so dependency
# downloads are skipped when only application code changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build a statically-linked binary with CGO disabled so it runs on any
# Alpine image without needing gcc or libc-dev. The deployment version is
# injected as 1.0.<commit-count> so each new commit produces an incremental
# version in the control panel.
RUN VERSION="1.0.0"; \
    if [ -d .git ]; then \
        COUNT=$(git rev-list --count HEAD 2>/dev/null || echo 0); \
        if [ "$COUNT" -gt 0 ]; then VERSION="1.0.$COUNT"; fi; \
    fi; \
    CGO_ENABLED=0 GOOS=linux go build -ldflags "-X github.com/jedafe/japa/internal/control.BuildVersion=$VERSION" -o /japa-api ./cmd/api/

# Stage 2: minimal runtime image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tesseract-ocr tesseract-ocr-data-eng poppler-utils ffmpeg

WORKDIR /app

COPY --from=builder /japa-api /app/japa-api

EXPOSE 8080

ENTRYPOINT ["/app/japa-api"]
