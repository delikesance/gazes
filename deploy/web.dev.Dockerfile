FROM node:24-alpine
RUN npm install -g pnpm@11.25.0
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1
CMD ["sh", "-c", "pnpm install --frozen-lockfile --store-dir=/pnpm/store && node scripts/subtitle-assets.mjs && exec pnpm exec next dev --webpack --hostname 0.0.0.0 --port 3000"]
