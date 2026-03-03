# Argus AI — Production Deployment Guide

Push-to-deploy workflow: tag a release in `argus-backend` or `argus-frontend`,
and the server updates automatically via GitLab CI/CD.

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Server Prerequisites](#2-server-prerequisites)
3. [SSH Key Setup](#3-ssh-key-setup)
4. [GitLab CI/CD Variables](#4-gitlab-cicd-variables)
5. [First-Time Server Setup](#5-first-time-server-setup)
6. [SSL Certificate Setup](#6-ssl-certificate-setup)
7. [Push to Deploy](#7-push-to-deploy)
8. [Deployment Flow](#8-deployment-flow)
9. [Git Commands Reference](#9-git-commands-reference)
10. [Rollback](#10-rollback)
11. [Monitoring & Troubleshooting](#11-monitoring--troubleshooting)

---

## 1. Architecture Overview

```
                         +-----------+
                         |  GitLab   |
                         |  CI/CD    |
                         +-----+-----+
                               |
              +----------------+----------------+
              |                                 |
     argus-backend CI                  argus-frontend CI
     (verify → build →                 (verify → build →
      publish → deploy)                 publish → deploy)
              |                                 |
              +--------+  trigger  +------------+
                       |           |
                  argus-infra CI
                  (validate → migrate → deploy)
                       |
                       | SSH
                       v
              +------------------+
              | Production Server|
              |                  |
              |  Nginx (:443)    |
              |    ├─ Frontend   |
              |    ├─ Backend    |
              |    └─ gRPC-Web   |
              |  Kafka (x3)      |
              |  ClickHouse (x3) |
              |  PostgreSQL      |
              |  Redis           |
              |  MinIO           |
              |  LiveKit         |
              +------------------+
```

---

## 2. Server Prerequisites

| Requirement       | Minimum              | Recommended           |
|--------------------|---------------------|-----------------------|
| **RAM**           | 32 GB               | 64 GB                 |
| **CPU Cores**     | 16                  | 32                    |
| **Disk**          | 500 GB SSD          | 1 TB NVMe             |
| **Docker**        | 27+                 | Latest stable         |
| **Docker Compose**| v2 (plugin)         | Latest stable         |
| **OS**            | Ubuntu 22.04 LTS    | Ubuntu 24.04 LTS      |

### Resource Budget (from docker-compose.prod.yml)

| Service               | CPUs | Memory | Instances |
|------------------------|------|--------|-----------|
| Backend               | 4.0  | 4 GB   | 1         |
| Frontend              | 1.0  | 512 MB | 1         |
| Kafka                 | 2.0  | 2 GB   | 3         |
| ClickHouse            | 2.0  | 3 GB   | 3         |
| ClickHouse Keeper     | 0.5  | 512 MB | 3         |
| PostgreSQL            | 2.0  | 2 GB   | 1         |
| Redis                 | 1.0  | 1 GB   | 1         |
| MinIO                 | 1.0  | 1 GB   | 1         |
| LiveKit               | 1.0  | 512 MB | 1         |
| Nginx                 | 1.0  | 256 MB | 1         |
| **Total**             | **24.5** | **~27 GB** | **16** |

---

## 3. SSH Key Setup

The deploy pipeline SSHes into your server to restart containers. You need an
SSH key pair: the **private key** goes into GitLab, the **public key** goes onto
the server.

### Step 1 — Generate a dedicated deploy key

```bash
ssh-keygen -t ed25519 -f ~/.ssh/argus_deploy -N "" -C "argus-deploy-key"
```

This creates two files:
- `~/.ssh/argus_deploy` (private key — goes to GitLab)
- `~/.ssh/argus_deploy.pub` (public key — goes to the server)

### Step 2 — Add the public key to your server

```bash
# Copy public key to the production server
ssh-copy-id -i ~/.ssh/argus_deploy.pub deploy@YOUR_SERVER_IP

# Verify SSH works
ssh -i ~/.ssh/argus_deploy deploy@YOUR_SERVER_IP "docker ps"
```

> The `deploy` user must have permission to run `docker` commands (add to
> `docker` group: `sudo usermod -aG docker deploy`).

### Step 3 — Add the private key to GitLab

1. Open your GitLab group: **argus_ai_group** > **Settings** > **CI/CD**
2. Expand **Variables**
3. Click **Add variable**
4. Configure:

| Field         | Value                                     |
|---------------|-------------------------------------------|
| **Key**       | `DEPLOY_SSH_KEY`                          |
| **Value**     | Paste the entire contents of `~/.ssh/argus_deploy` |
| **Type**      | **File**                                  |
| **Protected** | Yes                                       |
| **Masked**    | Yes                                       |

> **Group-level variables** are inherited by all 3 repos. Set variables at the
> group level to avoid repetition.

### Step 4 — Verify the key is accessible

After setting the variable, check that pipelines can read it by viewing the
**deploy-production** job logs in any infra pipeline run.

---

## 4. GitLab CI/CD Variables

Set these at **Group level** (`argus_ai_group` > Settings > CI/CD > Variables)
so all 3 repos inherit them:

| Variable                  | Type     | Protected | Masked | Example                                    |
|---------------------------|----------|-----------|--------|--------------------------------------------|
| `DEPLOY_HOST`             | Variable | Yes       | No     | `203.0.113.50` or `server.argus.ai`        |
| `DEPLOY_USER`             | Variable | Yes       | No     | `deploy`                                   |
| `DEPLOY_SSH_KEY`          | **File** | Yes       | Yes    | Contents of `~/.ssh/argus_deploy`          |
| `REGISTRY_HOST`           | Variable | Yes       | No     | `registry.argus.ai`                        |
| `REGISTRY_USER`           | Variable | Yes       | Yes    | `registry-bot`                             |
| `REGISTRY_PASSWORD`       | Variable | Yes       | Yes    | (registry access token)                    |
| `INFRA_TRIGGER_TOKEN`     | Variable | Yes       | Yes    | (from argus-infra > Settings > CI/CD > Pipeline trigger tokens) |
| `JWT_SIGNING_KEY`         | Variable | Yes       | Yes    | `openssl rand -hex 32`                     |
| `POSTGRES_PASSWORD`       | Variable | Yes       | Yes    | (strong random password)                   |
| `MINIO_ROOT_PASSWORD`     | Variable | Yes       | Yes    | (strong random password)                   |
| `LIVEKIT_API_SECRET`      | Variable | Yes       | Yes    | (min 32 characters)                        |
| `BACKEND_PUBLIC_URL`      | Variable | Yes       | No     | `https://api.argus.ai`                     |
| `FRONTEND_ORIGIN`         | Variable | Yes       | No     | `https://app.argus.ai`                     |
| `NUXT_PUBLIC_API_BASE_URL`| Variable | Yes       | No     | `https://api.argus.ai`                     |

### Generating secrets

```bash
# JWT signing key (64-char hex)
openssl rand -hex 32

# PostgreSQL password
openssl rand -base64 24

# MinIO root password
openssl rand -base64 24

# LiveKit API secret (min 32 chars)
openssl rand -base64 32
```

### Creating the INFRA_TRIGGER_TOKEN

1. Go to **argus-infra** > **Settings** > **CI/CD**
2. Expand **Pipeline trigger tokens**
3. Click **Add trigger** with description: `backend/frontend deploy trigger`
4. Copy the token and save it as `INFRA_TRIGGER_TOKEN` at the group level

---

## 5. First-Time Server Setup

SSH into your production server and run these commands once:

```bash
# 1. Create the deployment directory
sudo mkdir -p /opt/argus
sudo chown deploy:deploy /opt/argus
cd /opt/argus

# 2. Clone the infra repo (for compose files, configs, migrations)
git clone git@gitlab.com:argus_ai_group/argus-infra.git .

# 3. Create the production .env file
cp docker/.env.example docker/.env

# 4. Edit .env with real production values
nano docker/.env
```

Edit `docker/.env` with your actual secrets:

```env
# Image versions (managed by CI/CD — leave as 'latest' for first deploy)
BACKEND_TAG=latest
FRONTEND_TAG=latest

# Production URLs
BACKEND_PUBLIC_URL=https://api.argus.ai
FRONTEND_ORIGIN=https://app.argus.ai

# Secrets (must match GitLab CI/CD variables)
JWT_SIGNING_KEY=<output of: openssl rand -hex 32>
POSTGRES_PASSWORD=<output of: openssl rand -base64 24>
MINIO_ROOT_PASSWORD=<output of: openssl rand -base64 24>
LIVEKIT_API_SECRET=<output of: openssl rand -base64 32>
LIVEKIT_API_KEY=argus-prod-api-key

# Logging
LOG_LEVEL=info
```

```bash
# 5. Log into the container registry
docker login registry.argus.ai -u registry-bot

# 6. Start the full stack (first time — will pull all images)
cd /opt/argus/docker
docker compose -f docker-compose.yaml -f docker-compose.prod.yml up -d

# 7. Verify all services are healthy
docker compose ps
```

---

## 6. SSL Certificate Setup

### Option A — Let's Encrypt (production)

Before Nginx can serve HTTPS, you need certificates. The Certbot sidecar
handles automatic renewal, but the first certificate must be obtained manually.

```bash
cd /opt/argus/docker

# 1. Start nginx on port 80 only (for ACME challenge)
#    Temporarily comment out SSL lines in nginx.conf, or:
docker compose -f docker-compose.yaml -f docker-compose.prod.yml \
  run --rm certbot certonly \
    --webroot -w /var/www/certbot \
    -d app.argus.ai \
    -d api.argus.ai \
    --email admin@argus.ai \
    --agree-tos \
    --no-eff-email

# 2. Copy certs to the expected location (if using Let's Encrypt default path)
#    The certbot-certs volume maps /etc/letsencrypt → /etc/nginx/certs
#    Symlink or copy:
docker compose exec certbot sh -c \
  "ln -sf /etc/letsencrypt/live/app.argus.ai/fullchain.pem /etc/letsencrypt/fullchain.pem && \
   ln -sf /etc/letsencrypt/live/app.argus.ai/privkey.pem /etc/letsencrypt/privkey.pem"

# 3. Restart nginx to pick up new certificates
docker compose -f docker-compose.yaml -f docker-compose.prod.yml \
  restart nginx
```

After this, Certbot automatically renews certificates every 12 hours
(renewal only happens when within 30 days of expiry).

### Option B — Self-signed (development / staging)

```bash
cd /opt/argus

# Generate self-signed cert (valid 1 year)
mkdir -p certs
openssl req -x509 -nodes -days 365 \
  -newkey rsa:2048 \
  -keyout certs/privkey.pem \
  -out certs/fullchain.pem \
  -subj "/CN=localhost"

# Mount this directory as the certs volume in docker-compose
```

---

## 7. Push to Deploy

Once the server is set up and GitLab variables are configured, deployment
is fully automatic:

### Deploy the backend

```bash
cd argus-backend

# Make your changes, commit
git add .
git commit -m "feat: implement new detection algorithm"

# Tag a release — this triggers the full pipeline
git tag v1.0.0
git push origin main --tags
```

### Deploy the frontend

```bash
cd argus-frontend

# Make your changes, commit
git add .
git commit -m "feat: add exam dashboard redesign"

# Tag a release
git tag v1.0.0
git push origin main --tags
```

### Deploy infrastructure changes

```bash
cd argus-infra

# Update compose files, nginx config, migrations, etc.
git add .
git commit -m "infra: increase backend memory limit to 6GB"
git push origin main
```

> **Tags trigger automatic deployment.** Pushes to `main` without a tag will
> run validation but require manual approval to deploy.

---

## 8. Deployment Flow

### Backend / Frontend Release

```
Developer: git tag v1.2.0 && git push origin main --tags
    │
    ├─ GitLab CI (argus-backend or argus-frontend)
    │   ├─ verify:  lint → vet/typecheck → test (+ PostgreSQL) → security scan
    │   ├─ build:   compile binary + Docker image
    │   ├─ publish: docker push registry.argus.ai/argus/backend:v1.2.0
    │   └─ deploy:  trigger argus-infra pipeline
    │       └─ passes BACKEND_TAG=v1.2.0 (or FRONTEND_TAG=v1.2.0)
    │
    ├─ GitLab CI (argus-infra) — triggered automatically
    │   ├─ validate:  docker compose config --quiet
    │   ├─ migrate:   run SQL migrations (manual gate)
    │   └─ deploy:    SSH into server
    │       ├─ scp docker-compose.yaml → /opt/argus/
    │       ├─ docker compose pull backend frontend
    │       ├─ docker compose up -d --no-deps --wait backend
    │       └─ docker compose up -d --no-deps --wait frontend
    │
    └─ Done. Zero-downtime rolling restart complete.
```

### Infrastructure Changes

```
Developer: git push origin main (argus-infra)
    │
    ├─ validate-compose: docker compose config --quiet
    ├─ run-pg-migrations: (manual) apply SQL to PostgreSQL
    ├─ run-ch-migrations: (manual) apply SQL to ClickHouse
    └─ deploy-production: (manual or auto) SSH rolling deploy
```

---

## 9. Git Commands Reference

### Initial setup (clone all repos)

```bash
# Clone all 3 repos
git clone git@gitlab.com:argus_ai_group/argus-backend.git
git clone git@gitlab.com:argus_ai_group/argus-frontend.git
git clone git@gitlab.com:argus_ai_group/argus-infra.git
```

### Daily workflow

```bash
# Backend
cd argus-backend
git checkout main && git pull
git checkout -b feat/my-feature
# ... make changes ...
git add -A && git commit -m "feat: description"
git push -u origin feat/my-feature
# Create MR in GitLab, merge to main

# When ready to deploy:
git checkout main && git pull
git tag v1.2.0
git push origin v1.2.0
```

### Push all repos at once

```bash
#!/bin/bash
# push-all.sh — Push all 3 repos to GitLab
for repo in argus-backend argus-frontend argus-infra; do
  echo "=== Pushing $repo ==="
  cd "$repo"
  git push origin main
  cd ..
done
```

### Tag a coordinated release

```bash
#!/bin/bash
# release.sh — Tag and deploy all services
VERSION=${1:?"Usage: ./release.sh v1.2.0"}

for repo in argus-backend argus-frontend; do
  echo "=== Tagging $repo $VERSION ==="
  cd "$repo"
  git tag "$VERSION"
  git push origin "$VERSION"
  cd ..
done

echo "Release $VERSION triggered. Monitor at:"
echo "  https://gitlab.com/argus_ai_group/argus-infra/-/pipelines"
```

---

## 10. Rollback

### Roll back to a previous version

```bash
# SSH into the server
ssh deploy@YOUR_SERVER_IP

cd /opt/argus/docker

# Roll back backend to a specific version
BACKEND_TAG=v1.1.0 docker compose -f docker-compose.yaml \
  -f docker-compose.prod.yml up -d --no-deps backend

# Roll back frontend to a specific version
FRONTEND_TAG=v1.0.9 docker compose -f docker-compose.yaml \
  -f docker-compose.prod.yml up -d --no-deps frontend

# Roll back both
BACKEND_TAG=v1.1.0 FRONTEND_TAG=v1.0.9 docker compose \
  -f docker-compose.yaml -f docker-compose.prod.yml up -d --no-deps backend frontend
```

### Emergency: revert the last deploy

```bash
ssh deploy@YOUR_SERVER_IP
cd /opt/argus/docker

# See recent image tags
docker images --format "{{.Repository}}:{{.Tag}}" | grep argus

# Restart with the previous tag
BACKEND_TAG=<previous-tag> docker compose up -d --no-deps --wait backend
```

---

## 11. Monitoring & Troubleshooting

### Check service health

```bash
# All services
docker compose ps

# Backend health
curl -f http://localhost:8080/healthz

# View backend logs (last 100 lines)
docker compose logs --tail=100 backend

# Follow frontend logs in real-time
docker compose logs -f frontend

# Check nginx access logs
docker compose logs nginx | tail -50
```

### Common issues

| Symptom | Cause | Fix |
|---------|-------|-----|
| Pipeline fails at `publish-image` | Registry credentials invalid | Update `REGISTRY_PASSWORD` in GitLab CI/CD vars |
| Pipeline fails at `deploy-production` | SSH key rejected | Re-add `DEPLOY_SSH_KEY` as **File** type variable |
| `502 Bad Gateway` from Nginx | Backend not healthy yet | Wait 30s for health check, or check `docker compose logs backend` |
| Certbot renewal fails | Port 80 blocked | Ensure firewall allows HTTP for ACME challenge |
| ClickHouse out of memory | Query too large | Check `docker stats`, increase memory limit in `docker-compose.prod.yml` |
| Kafka broker won't start | Disk full | Check `df -h`, clean old Docker images: `docker system prune -af` |

### Useful commands

```bash
# Resource usage per container
docker stats --no-stream

# Disk usage by Docker
docker system df

# Clean unused images/volumes (careful in production)
docker system prune --volumes

# Restart a single service without downtime
docker compose up -d --no-deps --wait <service>

# View all environment variables for a service
docker compose exec backend env

# Check certificate expiry
openssl s_client -connect app.argus.ai:443 -servername app.argus.ai 2>/dev/null | \
  openssl x509 -noout -dates
```

---

## File Reference

| File | Repo | Purpose |
|------|------|---------|
| `.gitlab-ci.yml` | All 3 repos | Activates CI/CD (includes `ci/gitlab-ci.yml`) |
| `ci/gitlab-ci.yml` | All 3 repos | Full pipeline definition |
| `docker/docker-compose.yaml` | argus-infra | Base service definitions (16 services) |
| `docker/docker-compose.prod.yml` | argus-infra | Production overlay (resource limits, nginx, certbot) |
| `docker/.env.example` | argus-infra | Template for environment variables |
| `nginx/nginx.conf` | argus-infra | Reverse proxy config (TLS, gRPC-Web, rate limiting) |
| `migrations/postgres/*.sql` | argus-infra | PostgreSQL schema migrations |
| `migrations/clickhouse/*.sql` | argus-infra | ClickHouse analytics migrations |
| `Dockerfile` | backend, frontend | Multi-stage production Docker images |
