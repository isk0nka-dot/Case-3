# Argus AI — Operations & Troubleshooting Guide

**Аудитория:** Барлық инженерлер мен DevOps командасы.  
**Мақсат:** Бір жерін өзгерткенде басқасы сынбасын — осы нұсқаулық барлық критикалық байланыстарды, дұрыс деплой тәртібін, және жиі кездесетін мәселелерді сипаттайды.

---

## 📐 Жүйенің критикалық байланыстары (Coupling Map)

Мына диаграмма "мұны өзгерткенде ананы да тексер" деп ескертеді:

```
argus-frontend/ci/gitlab-ci.yml
        │── npm run build → .output/ → Dockerfile COPY .output
        │── rsync excludes → .output/server/node_modules ЖОҒАЛМАСЫН
        └── ARGUS_DEPLOY_HOST, ARGUS_DEPLOY_USER, ARGUS_DEPLOY_SSH_KEY

argus-backend/ci/gitlab-ci.yml
        │── go build → binary → Dockerfile
        └── ARGUS_DEPLOY_HOST, ARGUS_DEPLOY_USER, ARGUS_DEPLOY_SSH_KEY

argus-infra/docker/docker-compose.yaml
        │── healthcheck секциялары → CI deploy "healthy" болғанда ғана аяқталады
        │── /opt/argus-ai/argus-infra/migrations/clickhouse — init.sql + 00X файлдары ТӘРТІППЕН орындалады
        │── /opt/argus-ai/argus-backend және /opt/argus-ai/argus-frontend бөлек sync болады
        └── volumes: docker compose down -v → БАРЛЫҚ ДЕРЕКТЕР ЖОЙЫЛАДЫ

nginx конфигурациясы (Docker nginx, argus-network ішінде)
        └── /api/ → backend:8080, / → frontend:3000, /argus.proctoring → backend:50051
```

---

## 🚨 СЫНУЫ МҮМКІН — Критикалық ережелер

### 1. ClickHouse миграциялар (ӨТЕ МАҢЫЗДЫ)

**Мәселе:** Миграция файлдары алфавиттік тәртіппен орындалады. Кесте болмаса — келесі миграция сынады.

**Ережелер:**
- `init.sql` ең басында `USE argus_analytics;` болуы КЕРЕК
- Жаңа миграция файлы: `000006_...up.sql` формат (алдыңғы нөмірден +1)
- Миграцияда `IF NOT EXISTS` / `IF EXISTS` қолдан — идемпотентті болуы керек
- `RENAME TABLE` → тек бос сервердегі жаңа кестеге. Деректі ескі кестеден migrate еткенде бағандар санын тексер

**Жаңа кестеге деректер тасымалдау:**
```sql
-- ДҰРЫС: бағандарды нақты санап жазу
INSERT INTO argus_analytics.new_table (col1, col2, col3)
SELECT col1, col2, col3 FROM argus_analytics.old_table;

-- ҚАТЕ: SELECT * — ескі және жаңа кестенің баған саны сәйкес келмесе сынады
INSERT INTO argus_analytics.new_table SELECT * FROM argus_analytics.old_table;
```

**Миграция файлын серверге көшіру:**
```bash
scp migrations/clickhouse/000006_new_migration.up.sql deploy@89.167.96.246:/opt/argus-ai/argus-infra/migrations/clickhouse/
# Содан кейін GitLab-та manual `run-migrations` job іске қос немесе:
# ssh deploy@89.167.96.246 "cd /opt/argus-ai/argus-infra/docker && docker compose exec ..."
```

---

### 2. Frontend деплой (rsync + Docker)

**Мәселе:** `rsync --exclude='node_modules'` `.output/server/node_modules` папкасын да алып тастайды.

**ДҰРЫС (қазіргі күй):**
```yaml
rsync -azO --delete \
  --exclude='/.git' \       # "/" алдында → тек түбір папка
  --exclude='/node_modules' \
  --exclude='/.npm-cache' \
  --exclude='/.nuxt' \
```

**ҚАТЕ (бұрынғы):**
```yaml
--exclude='node_modules'    # "/" жоқ → .output/server/node_modules де жойылады
```

**Dockerfile ескертулері:**
- `.output/` Git-те ЖОҚ — CI `npm run build` арқылы жасайды
- `COPY .output /app/.output` — `build-production` job артефакті rsync арқылы жеткізілген болуы керек
- `node:22-alpine` — Node нұсқасын өзгертсе Dockerfile та өзгерту керек

---

### 3. Backend деплой

**Мәселе:** Go бинарын немесе конфигурацияны өзгерткенде PostgreSQL миграциялары да тексерілуі керек.

**Ережелер:**
- `internal/transport/grpc/mapper.go` — proto өзгерген сайын mapper та өзгеру керек
- `deployments/config.yaml` — жаңа environment variable қосылса, `.env.example` та жаңарту керек
- Kafka topic атаулары: `argus.events.standard`, `argus.events.critical`, `argus.events.telemetry` — өзгертем десе consumer да өзгерту керек

---

### 4. Docker Compose томдары (Volumes)

> ⚠️ **`docker compose down -v` — БАРЛЫҚ ДЕРЕКТІ ЖОЯДЫ**

```bash
# ДҰРЫС — деректі сақтап тоқтату
docker compose down

# ҚАТЕ — PostgreSQL, ClickHouse, Kafka деректерін жояды
docker compose down -v      # тек жаңа сервер баптағанда немесе толық reset үшін ғана
```

**Volume тізімі (жоғалтпа):**
- `argus-postgres-data` — организациялар, пайдаланушылар, API кілттер
- `argus-clickhouse-1/2/3-data` — оқиғалар аналитикасы
- `argus-minio-data` — дәлелдемелер (forensic evidence)
- `argus-badger-data` — DLQ (dead letter queue)

---

### 5. Nginx конфигурациясы

**Файл орны:** `/opt/argus-ai/argus-infra/nginx/nginx.conf`

**Маршруттар:**
```
https://argusai.kz/            → frontend:3000
https://argusai.kz/api/        → backend:8080
https://argusai.kz/argus.proctoring → backend:50051
```

**SSL сертификаты:** Let's Encrypt, `certbot` контейнері арқылы жаңарады.
**Тексеру:** `docker compose -f docker-compose.yaml -f docker-compose.prod.yml logs nginx`

---

## 🔄 Дұрыс деплой тәртібі

### Frontend өзгерту

```bash
cd argus-frontend
# 1. Код өзгерт
# 2. Жергілікті тексер
npm install && npm run dev

# 3. Push — CI өзі build жасайды
git add . && git commit -m "feat: ..." && git push
# → GitLab: install → typecheck → build → deploy (~7 мин)
```

### Backend өзгерту

```bash
cd argus-backend
# 1. Код өзгерт
# 2. Тест жаса
make test
make lint

# 3. Proto өзгерсе — TypeScript types та жаңарту керек!
# argus-frontend/app/lib/proto/types.ts

# 4. Push
git add . && git commit -m "feat: ..." && git push
```

### Инфрақұрылым өзгерту (docker-compose, nginx, migrations)

```bash
cd argus-infra
# 1. Өзгерт
# 2. Серверге SCP арқылы жіберу немесе push → CI
git add . && git commit -m "infra: ..." && git push

# Manual job тәртібі:
# 1) sync-infra
# 2) қажет болса run-migrations
# 3) nginx/infra өзгерісі болса production compose reload
```

---

## 🔧 Жиі кездесетін мәселелер

### ClickHouse контейнері іске қосылмайды

**Белгілері:** `argus-clickhouse-1` → `Error` статусы

```bash
# Себебін қара
docker logs argus-clickhouse-1 | tail -30

# Жиі себеп 1: Кесте жоқ (миграция тәртібі бұзылған)
# → migrations/clickhouse файлдарын тәртіппен тексер

# Жиі себеп 2: Баған саны сәйкес емес (SELECT * with schema mismatch)
# → 000004_reliability_hardening.up.sql ішінде INSERT SELECT * → нақты бағандар жаз

# Толық тазалап қайта бастау:
docker compose down -v  # ⚠️ деректер жойылады
docker compose up -d
```

### Frontend: ERR_MODULE_NOT_FOUND

**Себебі:** `rsync` `.output/server/node_modules` папкасын жойды.

```bash
# Тексер
docker logs argus-frontend | grep "Cannot find package"

# Шешім: rsync exclude дұрыс жазылғанын тексер
# --exclude='/node_modules'  ← "/" болуы МІНДЕТТІ
```

### Backend: Database migration failed

```bash
docker logs argus-backend | grep -i "migration\|error"

# PostgreSQL миграцияларын қолмен орындау:
docker exec argus-postgres psql -U argus -d argus -f /migrations/001_init.sql
```

### Pipeline: ARGUS_DEPLOY_SSH_KEY permission denied

```bash
# GitLab → Settings → CI/CD → Variables тексер:
# ARGUS_DEPLOY_SSH_KEY — type: File
# ARGUS_DEPLOY_HOST    — 89.167.96.246
# ARGUS_DEPLOY_USER    — deploy
```

### Диск толып қалды (100%)

```bash
ssh root@89.167.96.246

# Тегін орын тексер
df -h

# Docker тазалау (контейнер данасы мен ескі image)
docker system prune -f
docker volume prune -f  # ⚠️ тек пайдаланылмайтын volumes жояды

# Логтарды тазалау
journalctl --vacuum-time=3d
truncate -s 0 /var/log/syslog
```

---

## 🏥 Жүйе денсаулығын тексеру

```bash
# Барлық контейнерлер статусы
ssh deploy@89.167.96.246 "cd /opt/argus-ai/argus-infra/docker && docker compose ps"

# Backend тексеру
curl -s https://argusai.kz/api/healthz
curl -s https://argusai.kz/api/readyz

# ClickHouse тексеру
curl -s http://89.167.96.246:8123/ping

# Nginx тексеру
ssh deploy@89.167.96.246 "cd /opt/argus-ai/argus-infra/docker && docker compose logs nginx --tail=50"

# SSL сертификат мерзімі
ssh deploy@89.167.96.246 "cd /opt/argus-ai/argus-infra/docker && docker compose run --rm certbot certificates"
```

---

## 🛠️ Апаттан қалпына келтіру (Disaster Recovery)

### Backend circuit breaker ашылды

```bash
# ClickHouse circuit breaker қалпына келтіру
curl -X POST https://argusai.kz/api/v1/admin/system-reset \
  -H "Authorization: Bearer <super_admin_jwt>" \
  -H "Content-Type: application/json" \
  -d '{"actions": ["reset_ch_breaker", "flush_overflow"]}'

# Kafka circuit breaker
curl -X POST https://argusai.kz/api/v1/admin/system-reset \
  -H "Authorization: Bearer <super_admin_jwt>" \
  -d '{"actions": ["reset_kafka_breaker"]}'
```

### Барлығын қайта бастау (толық reset, деректер сақталады)

```bash
ssh deploy@89.167.96.246
cd /opt/argus-ai/argus-infra/docker
docker compose restart
```

### Толық тазалап қайта бастау (деректер жойылады)

```bash
ssh deploy@89.167.96.246
cd /opt/argus-ai/argus-infra/docker
# GitLab-та reset-project manual job қолданған дұрыс.
# Егер қолмен істесең, тек Argus namespace-ін түсір:
docker compose down         # деректер сақталады
docker compose up -d
sleep 30
docker compose ps           # барлығы healthy екенін тексер
```

---

## 📋 Өзгерту алдындағы тізім (Checklist)

### Frontend өзгерткенде:
- [ ] `npm run build` жергілікті сәтті ме?
- [ ] `npm run lint` қатесіз ме?
- [ ] `.output/` Git-те БОЛМАУЫ керек (`.gitignore`-да)
- [ ] API URL өзгерсе — `NUXT_PUBLIC_API_URL` GitLab CI Variables та өзгерт

### Backend өзгерткенде:
- [ ] `make test` сәтті ме?
- [ ] Proto өзгерді ме → `argus-frontend/app/lib/proto/types.ts` жаңарт
- [ ] Жаңа env variable → `.env.example` жаңарт
- [ ] ClickHouse schema өзгерді ме → жаңа migration файл жаз (IF NOT EXISTS)

### Инфрақұрылым өзгерткенде:
- [ ] `docker-compose.yaml` volumes өзгерді ме → деректерді backup жаса
- [ ] Жаңа migration → файл атауы `000X_name.up.sql` тәртіпте ме?
- [ ] Nginx өзгерді ме → `docker run ... nginx -t` немесе CI validate job тексер

---

## 📞 Сервер қосылу деректері

```bash
# Production сервер
ssh root@89.167.96.246        # root (тек апатта)
ssh deploy@89.167.96.246      # deploy (күнделікті жұмыс)

# Жоба файлдары
/opt/argus-ai/argus-infra/docker/       # docker-compose.yaml
/opt/argus-ai/argus-backend/            # backend коды
/opt/argus-ai/argus-frontend/           # frontend коды
/opt/argus-ai/argus-infra/migrations/   # ClickHouse + PostgreSQL миграциялары
/opt/argus-ai/argus-infra/nginx/        # nginx конфигурациясы
```

---

*Соңғы жаңарту: 2026-03-19 | Argus AI Engineering*
