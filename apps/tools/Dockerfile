# Multi-stage Dockerfile for the JapaConnect Tools surface (apps/tools).
# SvelteKit SSR via adapter-node, served by Bun in production.
# Ref: §78 (Tools Web Surface), ADR-086.

# --- Build stage ---
FROM oven/bun:1.3 AS build

WORKDIR /app

# Copy lockfile + package.json first for cacheable install layer.
COPY package.json bun.lock* ./
RUN bun install --frozen-lockfile || bun install

# Copy the rest of the source and build.
COPY . .
RUN bun run build

# --- Runtime stage ---
FROM oven/bun:1.3-slim AS runtime

WORKDIR /app

# Copy the built output and production dependencies.
COPY --from=build /app/build ./build
COPY --from=build /app/package.json ./
COPY --from=build /app/bun.lock* ./
COPY --from=build /app/node_modules ./node_modules

ENV NODE_ENV=production
ENV PORT=3005
EXPOSE 3005

# adapter-node produces a Node-compatible server; Bun runs it natively.
CMD ["bun", "build/index.js"]
