# ⚠️ СТРОГИЙ ГИД РАЗРАБОТЧИКА (STRICT DEVELOPER ONBOARDING)

**ВНИМАНИЕ ВСЕМ НОВЫМ ИНЖЕНЕРАМ:** Перед тем как писать код, делать ревью или менять архитектуру в ЛЮБОМ из репозиториев проекта Argus AI, вы **ОБЯЗАНЫ** прочитать этот документ. 

Любое дублирование кода, переписывание существующих компонентов, или непонимание микросервисной структуры будет строго отклоняться на этапе Code Review.

---

## 🏗️ 1. Архитектура и Карта Репозиториев
Старый монолитный репозиторий `argus_ai` **архивирован и больше не используется**. Проект теперь разделен на 4 независимых репозитория. Строго соблюдайте границы каждого из них:

1. ⚙️ **`argus-backend`** (Go, gRPC, HTTP)
   - Основной сервер (Event Collector, Inference Proxy, SaaS API).
   - *Где искать доки:* `argus-backend/README.md`
2. 💻 **`argus-frontend`** (Nuxt 4, Vue 3)
   - Дашборд администратора, портал проктора, страница студента.
   - *Где искать доки:* `argus-frontend/README.md`
3. 📦 **`argus-sdk`** (TypeScript)
   - Клиентская библиотека для браузера (MediaPipe, WebRTC, сбор телеметрии).
   - *Где искать доки:* `argus-sdk/README.md`
4. 🚀 **`argus-infra`** (Docker, CI/CD, Скрипты)
   - **Вы находитесь здесь.** Это центральный репозиторий для развертывания (Docker Compose, Kubernetes manifests), баз данных (ClickHouse, Postgres) и центральной документации.
   - *Где искать доки:* `ARCHITECTURE.md`, `DEPLOY.md`, `OPERATIONS.md`.

---

## 🛑 2. Строгие правила разработки (НЕТ ДУБЛИРОВАНИЮ!)

Каждый новый инженер, присоединяющийся к проекту, часто начинает переписывать существующий функционал, думая, что его нет. **ЭТО ЗАПРЕЩЕНО.**

1. **Не пишите велосипеды на фронтенде:**
   В `argus-frontend/composables` уже реализованы многие важные хуки (`useResilience`, `useLowSpecMode`, `useHealthGovernor`). Прежде чем писать логику для обработки плохого интернета или слабых ПК, проверьте существующий код.
2. **Не ищите AI в бэкенде:**
   Вся первичная аналитика (MediaPipe) происходит в браузере через `argus-sdk`. Go-бэкенд (`argus-backend`) **не содержит** Python-скриптов, YOLOv8 или OpenCV. Он лишь принимает готовые ивенты и отправляет их в Kafka/ClickHouse.
3. **Используйте глобальный поиск:**
   Если вы не нашли папку `cmd/` или воркер, это значит, что структура изменилась (монолит распилен). Убедитесь, что вы ищете в правильном репозитории (backend, sdk, frontend).
4. **Документируйте изменения:**
   Если вы меняете контракт API (proto-файлы), вы обязаны обновить `ARCHITECTURE.md` в `argus-infra`. Если вы добавляете новую переменную окружения — обновите `DEPLOY.md`.

## 📌 3. Процесс внесения изменений (Workflow)
1. Сначала клонируйте все 4 репозитория в одну рабочую папку.
2. Для локального запуска используйте `docker-compose.yaml` из `argus-infra/docker`.
3. Все PR (Pull Requests) должны проходить проверку линтером (встроено в CI каждого репо) и не дублировать код соседних микросервисов.

---

## 🎥 4. LiveKit Egress (Запись видео) — архитектура и правила
Функционал сохранения видеопотоков через LiveKit Egress настроен на **Track Composite Egress** (без тяжелого UI-рендеринга). Это не continuous full-session recording, а механизм для выборочной записи нужных треков студента.

### Компоненты

| Компонент | Где находится | Ответственность |
|-----------|----------------|-----------------|
| `livekit` | `docker/docker-compose.yaml` | WebRTC комнаты, Redis state, signed webhooks |
| `livekit-egress` | `docker/docker-compose.yaml` | Запись выбранных audio/video tracks в MP4 |
| `backend` | `argus-backend` | API start/stop, LiveKit SDK, webhook receiver |
| `postgres` | `migrations/postgres/000007_livekit_recordings.up.sql` | Таблица `livekit_recordings`, mapping `egress_id -> session/user/file` |
| `minio` | `docker/docker-compose.yaml` | S3-compatible storage для MP4 |

### Backend API

| Endpoint | Кто вызывает | Что делает |
|----------|--------------|------------|
| `POST /api/v1/media/recordings/start` | Frontend/admin | Запускает `StartTrackCompositeEgress`, сохраняет `egress_id` в PostgreSQL |
| `POST /api/v1/media/recordings/stop` | Frontend/admin | Останавливает egress по `egressId` |
| `POST /api/v1/livekit/webhook` | LiveKit server | Принимает signed `egress_ended`, обновляет `livekit_recordings` |

### Storage path

Backend просит LiveKit сохранить MP4 в:

```text
content/recordings/{roomName}/{sessionId}-{studentId}.mp4
```

Этот путь нужен только для удобной структуры MinIO. Логика приложения должна связывать запись с сессией только через `egress_id`, потому что webhook гарантированно возвращает `egress_id`.

### Обязательные настройки LiveKit

LiveKit server должен отправлять webhook в backend:

```yaml
webhook:
  api_key: ${LIVEKIT_API_KEY}
  urls:
    - http://backend:8080/api/v1/livekit/webhook
```

`livekit-egress` должен иметь `EGRESS_CONFIG_BODY` с `api_key`, `api_secret`, `ws_url`, Redis и S3/MinIO настройками. Backend дополнительно передает S3 output в каждом `TrackCompositeEgressRequest`.

### Правила развития

1. ✂️ **Selective Recording (Экономия ресурсов):**
   Не пишите непрерывные 1-2 часовые сессии тестирования! В будущем необходимо реализовать логику сохранения видео **ТОЛЬКО в момент обнаружения AI-нарушения** (например, телефон в кадре). Сохраняйте только 30-секундный фрагмент. Это снизит нагрузку на диск и CPU серверов в 10 раз.
2. 💻 **External CPU (Масштабирование):**
   Egress — крайне тяжелый процесс. При нагрузке свыше 100 одновременных студентов, контейнер `livekit-egress` **ОБЯЗАТЕЛЬНО** нужно выносить на отдельный выделенный сервер (Worker Node) с мощным процессором, иначе основной сервер платформы упадет от нехватки CPU.
3. 🔗 **Webhook + запись в БД:**
   LiveKit self-hosted должен иметь `webhook.urls` на `http://backend:8080/api/v1/livekit/webhook`. Backend сохраняет `egress_id` в PostgreSQL таблицу `livekit_recordings`, а событие `egress_ended` обновляет эту же запись по `egress_id`. Не парсите `session_id` из S3 filepath — это ненадежно.
4. 🪣 **S3 / MinIO output:**
   `livekit-egress` получает MinIO настройки через `EGRESS_CONFIG_BODY`, а backend дополнительно передает S3 output в каждом `TrackCompositeEgressRequest`. Это нужно, чтобы запись работала одинаково при локальном MinIO и будущих production S3-compatible хранилищах.
5. ✅ **Verification перед merge/deploy:**
   Для backend обязательно выполнить `go test ./...`. Для infra обязательно выполнить `docker compose -f docker/docker-compose.yaml config --quiet` с заполненными `MINIO_ROOT_PASSWORD`, `LIVEKIT_API_SECRET`, `JWT_SIGNING_KEY`.

**Нарушение этих правил приведет к немедленному отклонению вашего PR.**
