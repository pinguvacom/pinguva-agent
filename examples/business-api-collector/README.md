# Business API Collector Examples / Примеры сборщика бизнес-API

## English

These examples show how customer-side server code can send short API events to Pinguva.

Files:

- `node-express.js` — Node.js / Express middleware example.
- `php-middleware.php` — PHP helper example.
- `python-fastapi.py` — Python / FastAPI middleware example.

Required environment variables:

```text
PINGUVA_INGEST_URL=https://app.pinguva.com/api/business-api/ingest
PINGUVA_COLLECTOR_TOKEN=bapi_...
```

Do not send request bodies, response bodies, personal data, secrets or raw application logs.

## Русский

Эти примеры показывают, как серверный код клиента может отправлять в Pinguva короткие события по API.

Файлы:

- `node-express.js` — пример middleware для Node.js / Express.
- `php-middleware.php` — пример helper для PHP.
- `python-fastapi.py` — пример middleware для Python / FastAPI.

Нужные переменные окружения:

```text
PINGUVA_INGEST_URL=https://app.pinguva.com/api/business-api/ingest
PINGUVA_COLLECTOR_TOKEN=bapi_...
```

Не отправляйте тела запросов, тела ответов, персональные данные, секреты и сырые журналы приложения.
