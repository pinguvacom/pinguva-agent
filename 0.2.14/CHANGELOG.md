# Pinguva Agent 0.2.14 Changes

## Русский

- Добавлена агентская диагностика бизнес-API по стандартным access-log Nginx и Apache.
- Агент получает список важных маршрутов из обычного ответа `/api/agent/report`.
- Агент отправляет только минутные агрегаты: запросы, успешные ответы, бизнес-отказы, ошибки, задержки и маскированные источники.
- Динамические части маршрутов нормализуются в `:value`, чтобы не передавать реальные идентификаторы.
- Тела запросов, тела ответов, заголовки, cookies, query string, токены и сырые журналы не передаются.

## English

- Added agent-based Business API diagnostics from standard Nginx and Apache access logs.
- The agent receives watched routes through the regular `/api/agent/report` response.
- The agent sends minute-level aggregates only: requests, successful responses, business rejects, errors, latency and masked sources.
- Dynamic route segments are normalized to `:value` to avoid sending real identifiers.
- Request bodies, response bodies, headers, cookies, query strings, tokens and raw logs are not transmitted.
