# JapaConnect production image (SSR via adapter-node).
# The adapter-node runtime serves SSR. Port is taken from the PORT env var.
FROM oven/bun:1.3 AS base

WORKDIR /app

# Install with a frozen lockfile for reproducible builds.
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

# Build the SvelteKit SSR server into ./build
COPY . .
RUN bun run build

ENV PORT=3003
EXPOSE 3003

# adapter-node emits a Node-compatible server entry at build/index.js
CMD ["bun", "./build/index.js"]
