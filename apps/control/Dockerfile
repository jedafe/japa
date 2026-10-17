# syntax=docker/dockerfile:1.7-labs
# Control Panel — SvelteKit SPA (adapter-static). Build context is the
# repository root so bun can resolve workspace:* dependencies.
FROM oven/bun:1.4 AS build
WORKDIR /repo

COPY --parents package.json bun.lock apps/*/package.json packages/*/package.json ./
# --ignore-scripts: only manifests are present here, so workspace lifecycle
# hooks (svelte-kit sync, the extension's `wxt prepare`) cannot run yet.
RUN bun install --frozen-lockfile --ignore-scripts

COPY . .
# Generate every app's .svelte-kit/tsconfig.json — shared packages extend app
# tsconfigs (packages/landing-blocks -> apps/main), so syncing one app is not enough.
RUN bun run --filter './apps/*' prepare

# PUBLIC_* values are inlined by vite at build time.
ARG PUBLIC_JC_API_URL
ARG PUBLIC_JC_ENV
ARG PUBLIC_JC_GLITCHTIP_DSN
ENV PUBLIC_JC_API_URL=${PUBLIC_JC_API_URL} \
    PUBLIC_JC_ENV=${PUBLIC_JC_ENV} \
    PUBLIC_JC_GLITCHTIP_DSN=${PUBLIC_JC_GLITCHTIP_DSN}

WORKDIR /repo/apps/control
RUN bun run build

FROM oven/bun:1.4-slim AS runtime
WORKDIR /srv
ENV NODE_ENV=production \
    PORT=3002

COPY --from=build /repo/apps/control/build ./app
COPY docker/spa-server.mjs ./spa-server.mjs

USER bun

EXPOSE 3002
CMD ["bun", "spa-server.mjs"]
