# Argus AI — Service Level Objectives (SLO)

## Availability SLO

| Service | Target | Measurement Window |
|---|---|---|
| External API (`/api/v1/external/*`) | 99.5% uptime | 30-day rolling |
| Event Ingestion (gRPC `:50051`) | 99.9% uptime | 30-day rolling |
| Frontend Dashboard | 99.0% uptime | 30-day rolling |
| LiveKit WebRTC | 99.0% uptime | 30-day rolling |

## Latency SLO

| Endpoint | p50 | p95 | p99 |
|---|---|---|---|
| `POST /api/v1/external/sessions` | < 200ms | < 500ms | < 1s |
| `POST /api/v1/external/events` | < 100ms | < 300ms | < 800ms |
| `GET /api/v1/external/sessions/{id}/report` | < 500ms | < 2s | < 5s |
| gRPC IngestBatch | < 50ms | < 200ms | < 500ms |
| gRPC Heartbeat | < 30ms | < 100ms | < 300ms |

## Throughput SLO

| Metric | Target |
|---|---|
| Concurrent proctoring sessions | ≥ 500 per node |
| Event ingestion rate | ≥ 5,000 events/sec |
| Kafka write latency (p95) | < 50ms |
| ClickHouse flush interval | ≤ 5s |

## Error Budget

- **Availability budget**: 3.65 hours/month (99.5%)
- **Ingestion error budget**: 43 minutes/month (99.9%)
- Budget burn alert: fires at 2× burn rate for 1 hour

## Alert Thresholds

| Alert | Threshold | Severity |
|---|---|---|
| HTTP 5xx error rate | > 5% for 2m | warning |
| HTTP 5xx error rate | > 10% for 1m | critical |
| Ingestion drop rate | > 10/s for 2m | warning |
| p95 latency | > 2s for 5m | warning |
| p99 latency | > 5s for 2m | critical |
| Disk usage | > 85% | warning |
| Disk usage | > 95% | critical |

## Backup SLO

| Data | Backup Frequency | Retention | RTO | RPO |
|---|---|---|---|---|
| PostgreSQL | Daily at 02:00 | 14 days | < 1h | < 24h |
| ClickHouse DDL | Daily at 02:00 | 14 days | < 2h | < 24h |
| MinIO evidence | Continuous mirror to backup bucket | 30 days | < 4h | < 1h |

## Incident Response

| Severity | Response Time | Resolution Target |
|---|---|---|
| P1 (all sessions affected) | 15 minutes | 2 hours |
| P2 (partial service degradation) | 30 minutes | 4 hours |
| P3 (degraded performance) | 2 hours | 24 hours |
| P4 (minor issue) | 8 hours | 72 hours |
