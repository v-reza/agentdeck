# AgentDeck frontend image — Vite dev server (ARCHITECTURE 18.2).
#
# This is the local development stack, so the image runs `vite` and the source is
# bind-mounted by compose for hot reload. A production image would run
# `npm run build` and serve `dist/` from a static host instead.

FROM node:22-alpine
WORKDIR /app

# Lockfile first so a source-only change reuses the dependency layer.
COPY package.json package-lock.json ./
RUN npm ci

COPY . .

EXPOSE 5173
# --host 0.0.0.0 is required: the default binds to loopback, which is not
# reachable from outside the container.
CMD ["npx", "vite", "--host", "0.0.0.0", "--port", "5173"]
