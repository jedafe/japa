# syntax=docker/dockerfile:1.7
# Admin Panel — SvelteKit SSR (adapter-node). Build context is the repository root
# so bun can resolve workspace:* dependencies (packages/ui, packages/api-url).
FROM oven/bun:1.4 AS build
WORKDIR /repo

# Install all workspaces from the lockfile. --parents keeps each package.json
# in place so bun sees every workspace manifest.
COPY --parents package.json bun.lock apps/*/package.json packages/*/package.json ./
RUN bun install --frozen-lockfile

COPY . .

# PUBLIC_* values are inlined by vite at build time (they are build args, not
# runtime env). PRIVATE_* values are read at runtime by the SSR server.
ARG PUBLIC_JC_API_URL
ARG PUBLIC_JC_WS_URL
ARG PUBLIC_JC_ENV
ARG PUBLIC_JC_SITE_URL
ENV PUBLIC_JC_API_URL=${PUBLIC_JC_API_URL} \
    PUBLIC_JC_WS_URL=${PUBLIC_JC_WS_URL} \
    PUBLIC_JC_ENV=${PUBLIC_JC_ENV} \
    PUBLIC_JC_SITE_URL=${PUBLIC_JC_SITE_URL}

WORKDIR /repo/apps/admin
RUN bun run build

FROM oven/bun:1.4-slim AS runtime
WORKDIR /repo
ENV NODE_ENV=production \
    HOST=0.0.0.0 \
    PORT=3001

COPY --from=build /repo/package.json /repo/bun.lock ./
COPY --from=build /repo/node_modules ./node_modules
COPY --from=build /repo/packages ./packages
COPY --from=build /repo/apps/admin ./apps/admin

EXPOSE 3001
CMD ["bun", "apps/admin/build/index.js"]
