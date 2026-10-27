# syntax=docker/dockerfile:1.7-labs
# Japa Academy — SvelteKit SSR (adapter-node). Build context is the repository root
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
ARG PUBLIC_MAIN_ORIGIN
ARG PUBLIC_JC_UMAMI_URL
ARG PUBLIC_JC_UMAMI_SITE_ID_ACADEMY
ENV PUBLIC_JC_API_URL=${PUBLIC_JC_API_URL} \
    PUBLIC_MAIN_ORIGIN=${PUBLIC_MAIN_ORIGIN} \
    PUBLIC_JC_UMAMI_URL=${PUBLIC_JC_UMAMI_URL} \
    PUBLIC_JC_UMAMI_SITE_ID_ACADEMY=${PUBLIC_JC_UMAMI_SITE_ID_ACADEMY}

WORKDIR /repo/apps/academy
RUN bun run build

FROM oven/bun:1.4-slim AS runtime
WORKDIR /repo
ENV NODE_ENV=production \
    HOST=0.0.0.0 \
    PORT=3003

# Production-only deps — the build stage's node_modules carries vite,
# typescript, eslint, etc. Reinstall from manifests with --production;
# the adapter-node bundle only needs runtime deps at serve time.
COPY --parents package.json bun.lock apps/*/package.json packages/*/package.json ./
RUN bun install --frozen-lockfile --ignore-scripts --production --filter './apps/academy'

COPY --from=build /repo/packages ./packages
COPY --from=build /repo/apps/academy/build ./apps/academy/build
COPY --from=build /repo/apps/academy/package.json ./apps/academy/package.json

USER bun

EXPOSE 3003
CMD ["bun", "apps/academy/build/index.js"]
