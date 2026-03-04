# =============================================================================
# argus-frontend — Multi-stage Dockerfile
# =============================================================================
#
# Stage 1 (builder): Installs dependencies and produces a static Nuxt build.
# Stage 2 (runtime): Serves the SPA via a minimal Node.js image.
#
# Build args:
#   NUXT_PUBLIC_API_BASE_URL  Backend API URL baked into the build.
#                              Can be overridden at runtime via environment.
#
# Usage:
#   docker build \
#     --build-arg NUXT_PUBLIC_API_BASE_URL=https://api.argus.ai \
#     -t argus/frontend:latest .
#
#   docker run -p 3000:3000 \
#     -e NUXT_PUBLIC_API_BASE_URL=https://api.argus.ai \
#     argus/frontend:latest
# =============================================================================

# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM node:22-alpine AS builder

WORKDIR /app

# Build argument — baked into the static output.
ARG NUXT_PUBLIC_API_BASE_URL=http://localhost:8080
ENV NUXT_PUBLIC_API_BASE_URL=${NUXT_PUBLIC_API_BASE_URL}

# Install dependencies first (layer cache).
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts

# Copy application source.
COPY . .

# Build the Nuxt application.
RUN npm run build

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM node:22-alpine AS runtime

WORKDIR /app

# Create non-root user for security.
RUN addgroup -S argus && adduser -S argus -G argus

# Copy only the built output from the builder stage.
COPY --from=builder --chown=argus:argus /app/.output /app/.output

USER argus

# Nuxt server listens on port 3000 by default.
EXPOSE 3000

# Runtime env vars (override NUXT_PUBLIC_API_BASE_URL if needed).
ENV NODE_ENV=production
ENV PORT=3000
ENV HOST=0.0.0.0

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:3000/ || exit 1

CMD ["node", "/app/.output/server/index.mjs"]
