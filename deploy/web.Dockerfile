FROM node:24-alpine AS build
WORKDIR /app
RUN npm install -g pnpm@11.25.0
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN --mount=type=cache,target=/pnpm/store pnpm install --frozen-lockfile --store-dir=/pnpm/store
COPY web/ ./
# next.config rewrites /api/* to BACKEND_URL at build time, so the value is baked into the image: compose passes the
# same one it sets at runtime (build.args), and the default matches it, so the two cannot drift.
ARG BACKEND_URL=http://gluetun:8090
ENV NEXT_TELEMETRY_DISABLED=1 BACKEND_URL=$BACKEND_URL
RUN node scripts/subtitle-assets.mjs && pnpm exec next build --webpack

FROM node:24-alpine
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 HOSTNAME=0.0.0.0 PORT=3000
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
COPY --from=build --chown=node:node /app/public ./public
USER node
EXPOSE 3000
CMD ["node", "server.js"]
