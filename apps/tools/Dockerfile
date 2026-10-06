# syntax=docker/dockerfile:1.7
# Tools — SvelteKit SSR (adapter-node). Build context is the repository root
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
ARG PUBLIC_MAIN_ORIGIN
ENV PUBLIC_JC_API_URL=${PUBLIC_JC_API_URL} \
    PUBLIC_MAIN_ORIGIN=${PUBLIC_MAIN_ORIGIN}

WORKDIR /repo/apps/tools
RUN bun run build

FROM oven/bun:1.4-slim AS runtime
WORKDIR /repo
ENV NODE_ENV=production \
    HOST=0.0.0.0 \
    PORT=3005

COPY --from=build /repo/package.json /repo/bun.lock ./
COPY --from=build /repo/node_modules ./node_modules
COPY --from=build /repo/packages ./packages
COPY --from=build /repo/apps/tools ./apps/tools

EXPOSE 3005
CMD ["bun", "apps/tools/build/index.js"]
