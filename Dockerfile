# Prebuilt Go/Quasar artifacts only; the only compile step is installing the
# Soulseek bridge's Node dependencies in a throwaway stage.
FROM node:20-bookworm-slim AS bridge

WORKDIR /bridge
RUN apt-get update \
 && apt-get install -y --no-install-recommends python3 make g++ libsecret-1-dev \
 && rm -rf /var/lib/apt/lists/*

COPY server/soulseek-bridge/package.json ./
RUN npm install --omit=dev --no-audit --no-fund

COPY server/soulseek-bridge/bridge.mjs ./

FROM node:20-bookworm-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates \
      libchromaprint-tools \
      libsecret-1-0 \
      ffmpeg \
      nginx-light \
 && rm -rf /var/lib/apt/lists/* /var/www/html

WORKDIR /app

COPY listune-server-linux-amd64 /usr/local/bin/listune-server
COPY dist/spa /app/public
COPY server/data /app/data
COPY deploy/nginx.conf /etc/nginx/nginx.conf
COPY deploy/entrypoint.sh /usr/local/bin/docker-entrypoint.sh
COPY --from=bridge /bridge /app/soulseek-bridge

RUN chmod 0755 /usr/local/bin/listune-server /usr/local/bin/docker-entrypoint.sh \
 && mkdir -p /app/data/gtokens /app/data/library \
 && rm -f /etc/nginx/sites-enabled/default

EXPOSE 3322

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
