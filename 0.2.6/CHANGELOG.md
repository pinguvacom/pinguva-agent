# Pinguva Agent 0.2.6 Changes

## Русский

- Добавлена локальная диагностика нагрузки для коробочного Bitrix24.
- Новый ограниченный systemd timer раз в минуту собирает агрегаты access log и
  технические показатели MySQL на сервере клиента.
- В Pinguva передаются только нормализованные маршруты, числа запросов, 5xx,
  маскированные источники и фиксированные категории запросов.
- Сырые журналы, query string, cookies, заголовки авторизации, SQL-текст и
  SQL-значения не покидают сервер клиента.

## English

- Added local load diagnostics for self-hosted Bitrix24.
- A new bounded systemd timer collects access-log aggregates and MySQL
  technical metrics once a minute on the customer host.
- Pinguva receives only normalized routes, request counts, 5xx, masked sources
  and fixed query categories.
- Raw logs, query strings, cookies, authorization headers, SQL text and SQL
  values never leave the customer host.
