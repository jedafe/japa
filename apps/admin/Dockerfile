# syntax=docker/dockerfile:1.7-labs
# Admin Panel — SvelteKit SSR (adapter-node). Build context is the repository root
# so bun can resolve workspace:* dependencies (packages/ui, packages/api-url).
FROM oven/bun:1.4 AS build
WORKDIR /repo

# Install all workspaces from the lockfile. --parents keeps each package.json
# in place so bun sees every workspace manifest.
COPY --parents package.json bun.lock apps/*/package.json packages/*/package.json ./
# --ignore-scripts: only manifests are present here, so workspace lifecycle
# hooks (svelte-kit sync, the extension's `wxt prepare`) cannot run yet.
RUN bun install --frozen-lockfile --ignore-scripts

COPY . .
# Generate every app's .svelte-kit/tsconfig.json — shared packages extend app
# tsconfigs (packages/landing-blocks -> apps/main), so syncing one app is not enough.
RUN bun run --filter './apps/*' prepare

# PUBLIC_* values are inlined by vite at build time (they are build args, not
# runtime env). PRIVATE_* values are read at runtime by the SSR server.
ARG PUBLIC_JC_API_URL
ARG PUBLIC_JC_WS_URL
ARG PUBLIC_JC_ENV
ARG PUBLIC_JC_SITE_URL
ARG PUBLIC_JC_GLITCHTIP_DSN
ENV PUBLIC_JC_API_URL=${PUBLIC_JC_API_URL} \
    PUBLIC_JC_WS_URL=${PUBLIC_JC_WS_URL} \
    PUBLIC_JC_ENV=${PUBLIC_JC_ENV} \
    PUBLIC_JC_SITE_URL=${PUBLIC_JC_SITE_URL} \
    PUBLIC_JC_GLITCHTIP_DSN=${PUBLIC_JC_GLITCHTIP_DSN}

WORKDIR /repo/apps/admin
RUN bun run build

FROM oven/bun:1.4-slim AS runtime
WORKDIR /repo
ENV NODE_ENV=production \
    HOST=0.0.0.0 \
    PORT=3001

# Production-only deps — the build stage's node_modules carries vite,
# typescript, eslint, etc. Reinstall from manifests with --production;
# the adapter-node bundle only needs runtime deps at serve time.
COPY --parents package.json bun.lock apps/*/package.json packages/*/package.json ./
RUN bun install --frozen-lockfile --ignore-scripts --production --filter './apps/admin'

COPY --from=build /repo/packages ./packages
COPY --from=build /repo/apps/admin/build ./apps/admin/build
COPY --from=build /repo/apps/admin/package.json ./apps/admin/package.json

USER bun

EXPOSE 3001
CMD ["bun", "apps/admin/build/index.js"]
