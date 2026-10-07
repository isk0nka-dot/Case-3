# =============================================================================
# argus-frontend — Pre-built Dockerfile
# =============================================================================
#
# .output/ is pre-built locally and committed to git.
# This image simply copies the pre-built output — no npm ci / nuxt build needed.
#
# Usage:
#   docker build -t argus/frontend:latest .
#   docker run -p 3000:3000 argus/frontend:latest
# =============================================================================

FROM node:22-alpine

WORKDIR /app

# Create non-root user for security.
RUN addgroup -S argus && adduser -S argus -G argus

# Copy pre-built Nuxt output from git (built locally before commit).
COPY --chown=argus:argus .output /app/.output

USER argus

# Nuxt server listens on port 3000 by default.
EXPOSE 3000

ENV NODE_ENV=production
ENV PORT=3000
ENV HOST=0.0.0.0

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:3000/ || exit 1

CMD ["node", "/app/.output/server/index.mjs"]
