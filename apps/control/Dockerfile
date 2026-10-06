# syntax=docker/dockerfile:1.7
# Control Panel — SvelteKit SPA (adapter-static). Build context is the
# repository root so bun can resolve workspace:* dependencies.
FROM oven/bun:1.4 AS build
WORKDIR /repo

COPY --parents package.json bun.lock apps/*/package.json packages/*/package.json ./
RUN bun install --frozen-lockfile

COPY . .

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

EXPOSE 3002
CMD ["bun", "spa-server.mjs"]
