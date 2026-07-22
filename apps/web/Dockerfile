FROM node:24.18.0-alpine AS build

WORKDIR /workspace
COPY package.json package-lock.json ./
RUN npm ci

COPY index.html tsconfig.json vite.config.ts vitest.setup.ts ./
COPY src ./src
RUN npm run build

FROM caddy:2.11.4-alpine

COPY Caddyfile /etc/caddy/Caddyfile
COPY --from=build /workspace/dist /srv

EXPOSE 3000
