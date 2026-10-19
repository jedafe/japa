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
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w -X github.com/jedafe/japa/internal/control.BuildVersion=$VERSION" -o /japa-api ./cmd/api/

# Ops binaries for one-off execs on the deployed container — the api image
# has no Go toolchain, so these must be prebuilt: migrations on every
# update, seed/geo-seed/requeue for the fresh-prod launch sequence
# (docs/deployment-coolify.md).
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /japa-migrate ./cmd/migrate/ && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /japa-seed ./cmd/seed/ && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /japa-seed-geo-cities ./cmd/seed-geo-cities/ && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /japa-requeue-universities ./cmd/requeue-universities/

# Stage 2: minimal runtime image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tesseract-ocr tesseract-ocr-data-eng poppler-utils ffmpeg

# Run as an unprivileged user — the API should never be root inside the
# container. The media scratch dir (JC_MEDIA_ROOT default "data/media",
# resolved under WORKDIR) must be writable by that user.
RUN addgroup -S japa && adduser -S japa -G japa

WORKDIR /app

RUN mkdir -p /app/data/media && chown -R japa:japa /app

COPY --from=builder /japa-api /app/japa-api
COPY --from=builder /japa-migrate /app/japa-migrate
COPY --from=builder /japa-seed /app/japa-seed
COPY --from=builder /japa-seed-geo-cities /app/japa-seed-geo-cities
COPY --from=builder /japa-requeue-universities /app/japa-requeue-universities

# Migration files (FindMigrationsDir resolves ./migrations under WORKDIR)
# and the geo/university seed data seed-geo-cities + wikifacts read via
# db/seed/... relative paths.
COPY migrations/ /app/migrations/
COPY db/seed/ /app/db/seed/

USER japa

EXPOSE 8080

ENTRYPOINT ["/app/japa-api"]
