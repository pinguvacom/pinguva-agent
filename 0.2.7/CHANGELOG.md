# Pinguva Agent 0.2.7 Changes

## Русский

- Добавлена команда `bitrix24 bootstrap` для безопасного обновления уже
  настроенной локальной интеграции Bitrix24.
- Bootstrap работает только при существующем локальном конфигурационном файле,
  не меняет webhook и выбранные REST-профили.
- Добавлены стандартные ограниченные пути Nginx/Apache access log и root-owned
  таймер локальной диагностики раз в минуту.
- Агент получает выбранные важные маршруты только в ответе на собственный
  исходящий отчёт и включает их в агрегат даже вне общего Top-маршрутов.
- Сырые журналы, query string, cookies, заголовки авторизации, webhook, SQL-текст
  и SQL-значения не покидают сервер клиента.

## English

- Added `bitrix24 bootstrap` to safely upgrade an existing local Bitrix24
  integration.
- Bootstrap runs only when local configuration exists and does not change the
  webhook or selected REST profiles.
- Added bounded standard Nginx/Apache access-log paths and a root-owned local
  diagnostics timer that runs once a minute.
- The agent receives selected important routes only in the response to its own
  outbound report and keeps them in the aggregate outside the general Top list.
- Raw logs, query strings, cookies, authorization headers, webhooks, SQL text
  and SQL values never leave the customer server.
