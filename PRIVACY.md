# Privacy Notes

## English

Pinguva Agent sends only the technical data needed for monitoring. The published source code is intended to make this boundary reviewable.

The agent can send server telemetry such as CPU, memory, disk, network, uptime, and Ping. On Linux it can also send Disk I/O, watched-service status, and bounded configuration-change metadata. For a configured, compatible self-hosted Bitrix24 integration, it can send REST status and latency, safe result counts, minute-level REST and MySQL aggregates, SQL digests, safe normalized `SELECT` structure when redaction succeeds, and supported lock-wait counters.

The agent does not send configuration-file contents, Bitrix24 webhook URLs, MySQL passwords, OAuth or API tokens, CRM records, raw access logs, URL query parameters, cookies, authorization headers, original SQL, SQL values, HTTP request or response bodies, or arbitrary files. It does not accept inbound network connections or execute remote commands received from Pinguva.

The documented production configuration sends reports through outbound HTTPS to Pinguva. Bitrix24 webhooks and MySQL credentials remain on the customer server.

## Русский

Pinguva Agent отправляет в платформу только технические данные, необходимые для мониторинга.

Что агент отправляет:
- системные метрики сервера: CPU, память, диск, сеть, uptime, ping;
- для Linux: Disk I/O, watched services, контроль изменений конфигурации;
- для Bitrix24 в версии 0.2.6+: только флаг настройки, статус проверки REST,
  выбранные REST-профили, задержку методов, безопасный count и короткую ошибку
  после удаления секретов;
- для локальной диагностики Bitrix24 `0.2.12+`: минутные агрегаты динамических
  запросов и 5xx, нормализованные пути без query string, технические счётчики
  MySQL, ожидания блокировок при наличии совместимого Performance Schema и
  дельты SQL digest; безопасная структура `SELECT` передаётся только после
  удаления значений, иначе передаются только digest, категория и счётчики;
- metadata по watched config profiles:
  - путь;
  - тип записи;
  - факт существования;
  - время изменения;
  - размер;
  - SHA-256.

Что агент не отправляет:
- содержимое конфигурационных файлов;
- Bitrix24 webhook URL;
- OAuth token, access token, refresh token;
- CRM-данные и тела ответов Bitrix24;
- сырые access log и параметры URL;
- cookies и заголовки авторизации;
- исходный SQL и его значения;
- телефоны, email, комментарии, названия сделок и пользовательские поля;
- удалённые shell-команды;
- входящие сетевые сервисы;
- произвольные файлы с сервера.

Агент работает через исходящее HTTPS-соединение к контуру Pinguva.
