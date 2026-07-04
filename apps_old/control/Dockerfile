FROM oven/bun:1.3 AS base
WORKDIR /app
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile
COPY . .
RUN bun run build
EXPOSE 3002
CMD ["bun", "run", "preview"]
