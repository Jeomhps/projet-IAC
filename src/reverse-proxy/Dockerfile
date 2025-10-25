# Stage 1: Build the SPA
FROM node:20-alpine AS webbuild
WORKDIR /web

# Install deps first (cache-friendly)
# NOTE: build context is src/, so "web/..." is correct.
COPY web/package*.json ./
# Use npm ci if lockfile exists; otherwise npm install
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi

# Copy the app and build
COPY web ./
RUN npm run build

# Stage 2: Caddy serves the built SPA and proxies /api to the Flask API
FROM caddy:2-alpine
# Caddy config
COPY reverse-proxy/Caddyfile /etc/caddy/Caddyfile
# Built static files
COPY --from=webbuild /web/dist /srv/web
