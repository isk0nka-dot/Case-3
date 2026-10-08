# Argus AI - Local Development Setup

Бұл монорепозиторийде Argus AI жүйесінің барлық компоненттері жинақталған.

## 1. Локалды іске қосу (Docker арқылы)

Алдымен компьютеріңізге [Docker Desktop](https://www.docker.com/products/docker-desktop/) орнатыңыз және іске қосыңыз.

Содан кейін терминалды ашып, осы `Case-3` папкасының ішінде келесі команданы орындаңыз:

```bash
docker compose up -d --build
```

Бұл команда:
1. **Frontend (Nuxt)**: Дашбордты `http://localhost:3000` адресінде көтереді.
2. **Backend (Go)**: API серверін `http://localhost:8080` адресінде көтереді.
3. **AI Sidecar (Python)**: YOLOv8 және MediaPipe үшін `http://localhost:8091` адресінде көтеріледі.
4. **Инфрақұрылым**: Postgres, ClickHouse, Redis, Kafka, MinIO, LiveKit сервистерін көтереді және дерекқор миграцияларын автоматты түрде жасайды.

### Модельдерді жүктеу (Hackathon үшін)
AI Sidecar жұмыс істеуі үшін YOLOv8 және ArcFace модельдері қажет. Жоба ішінде `argus-backend/models` папкасы бар. Егер интернет жақсы болса, келесі скриптті іске қосып модельдерді жүктей аласыз:
```bash
cd argus-backend/ai-sidecar
bash scripts/download_models.sh
```

## 2. Дашбордты ашу

Docker барлығын көтергеннен кейін бразуерде ашыңыз:
👉 **[http://localhost:3000](http://localhost:3000)**

## 3. Жұмыс ортасын қорғау (Hackathon талабы)
Хакатонның 2.3 талабы бойынша (Alt+Tab, Win бұғаттау) біз арнайы Python клиентін немесе Electron қосымшасын дайындауымыз керек. Бұл браузермен бірге іске қосылатын кішігірім бағдарлама болады. (Скрипт `secure-client/` папкасында дайындалуда).

