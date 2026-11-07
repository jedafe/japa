# Render service — headless chromium behind the Go RenderFetcher
# contract. oven/bun carries Bun; playwright installs its own chromium
# build under /ms-playwright (PLAYWRIGHT_BROWSERS_PATH keeps it off the
# app layer).
#
# Build:  docker build -f services/render/Dockerfile -t japa-render .
# Run:    docker run -p 8300:8300 -e RENDER_AUTH_TOKEN=... japa-render
FROM oven/bun:1-debian

ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
    RENDER_PORT=8300

WORKDIR /app

COPY services/render/package.json ./package.json
RUN bun install --production
RUN bunx playwright install --with-deps chromium

COPY services/render/src ./src

USER bun
EXPOSE 8300
CMD ["bun", "run", "src/server.ts"]
