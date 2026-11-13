# Privacy Notes

## English

Pinguva Agent sends technical data for monitoring. The published source code is intended to make this boundary reviewable. This is not a zero-data or anonymous-data service: the base report includes hostname and server IP, and optional integrations can include selected local paths, route labels, database names and traffic-source summaries.

The agent can send server telemetry such as CPU, memory, disk, network, uptime, and Ping. On Linux it can also send Disk I/O, watched-service status, and configuration-change metadata including selected file paths, sizes, timestamps and SHA-256 fingerprints. For a configured, compatible self-hosted Bitrix24 integration, it can send REST status and latency, safe result counts, minute-level REST and MySQL aggregates, SQL digests, safe normalized `SELECT` structure when redaction succeeds, and supported lock-wait counters. For configured database diagnostics it can send MySQL, MariaDB, or PostgreSQL technical counters, database or endpoint metadata and safe query-group aggregates. For configured Business API monitoring it can send route labels and counters from local logs, numeric error-code counts, traffic-source summaries, and discovered local log paths and route names. IP-looking source values are abbreviated, but other source strings may be retained in the summary.

The agent does not send configuration-file contents, Bitrix24 webhook URLs, database passwords, source-application OAuth or API tokens, CRM records, raw access logs, URL query parameters, cookies, source-application authorization headers, original SQL, SQL values, HTTP request or response bodies, or arbitrary files. Its own agent authentication token is sent in the report's Bearer header. It does not accept inbound network connections or execute remote commands received from Pinguva.

The documented production configuration sends reports through outbound HTTPS to Pinguva. Agent authentication uses a Bearer token in the HTTPS header. Bitrix24 webhooks and database credentials remain on the customer server. The code also permits a custom HTTP server URL for development, so production administrators must explicitly configure HTTPS. See the [customer security review](./docs/en/SECURITY_REVIEW.md) for network flows, payload examples and verification steps.

## Русский

Pinguva Agent отправляет технические данные для мониторинга. Это не режим «ничего не передаём» и не анонимная телеметрия: базовый отчёт содержит hostname и IP сервера, а включённые интеграции могут передавать выбранные локальные пути, названия маршрутов и баз данных, сводки по источникам трафика.

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
- для диагностики СУБД `0.2.13+`: технические счётчики MySQL, MariaDB или
  PostgreSQL и безопасные агрегаты групп запросов;
- для мониторинга бизнес-API `0.2.14+`: маршруты, HTTP-методы, группы кодов
  ответа, количество запросов, задержку при наличии request time в access-log и
  сводки по источникам трафика; IP-подобные значения сокращаются, но другие
  строки `source` могут сохраняться; в `0.2.23+` - минутные количества числовых
  кодов ошибок; при обнаружении источников - локальные пути файлов и найденные
  безопасные названия маршрутов;
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
- пароли баз данных;
- OAuth/access/refresh токены наблюдаемых приложений;
- CRM-данные и тела ответов Bitrix24;
- сырые access log и параметры URL;
- query string, тела запросов и тела ответов API;
- cookies и заголовки авторизации наблюдаемых приложений;
- исходный SQL и его значения;
- телефоны, email, комментарии, названия сделок и пользовательские поля;
- удалённые shell-команды;
- входящие сетевые сервисы;
- произвольные файлы с сервера.

Штатная production-настройка использует исходящее HTTPS-соединение к Pinguva.
Токен самого агента передаётся в заголовке `Authorization: Bearer`. Код допускает
настройку HTTP-адреса для разработки, поэтому HTTPS нужно явно проверить при
развёртывании. Подробности для ИБ - в [описании потоков и данных](./docs/ru/SECURITY_REVIEW.md).
