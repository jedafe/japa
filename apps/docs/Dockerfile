# syntax=docker/dockerfile:1.7-labs
# Docs — SvelteKit SSR (adapter-node). Build context is the repository root
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
ARG PUBLIC_ADMIN_ORIGIN
ARG PUBLIC_DOCS_ORIGIN
ENV PUBLIC_JC_API_URL=${PUBLIC_JC_API_URL} \
    PUBLIC_MAIN_ORIGIN=${PUBLIC_MAIN_ORIGIN} \
    PUBLIC_ADMIN_ORIGIN=${PUBLIC_ADMIN_ORIGIN} \
    PUBLIC_DOCS_ORIGIN=${PUBLIC_DOCS_ORIGIN}

WORKDIR /repo/apps/docs
RUN bun run build

FROM oven/bun:1.4-slim AS runtime
WORKDIR /repo
ENV NODE_ENV=production \
    HOST=0.0.0.0 \
    PORT=3004

# Production-only deps — the build stage's node_modules carries vite,
# typescript, eslint, etc. Reinstall from manifests with --production;
# the adapter-node bundle only needs runtime deps at serve time.
COPY --parents package.json bun.lock apps/*/package.json packages/*/package.json ./
RUN bun install --frozen-lockfile --ignore-scripts --production

COPY --from=build /repo/packages ./packages
COPY --from=build /repo/apps/docs/build ./apps/docs/build
COPY --from=build /repo/apps/docs/package.json ./apps/docs/package.json

USER bun

EXPOSE 3004
CMD ["bun", "apps/docs/build/index.js"]
