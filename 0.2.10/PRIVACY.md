# Privacy Notes

## Русский

Pinguva Agent `0.2.10` передаёт только технические данные, необходимые для
мониторинга.

Агент передаёт:

- системные метрики сервера: CPU, память, диск, сеть, uptime и ping;
- для Linux: Disk I/O, выбранные сервисы и факт изменения конфигурационных
  файлов без их содержимого;
- для подключенного коробочного Bitrix24: агрегаты маршрутов и `5xx`,
  маскированные источники, счётчики потоков MySQL и фиксированные категории
  текущих запросов.

Агент не передаёт:

- содержимое конфигурационных файлов;
- webhook Bitrix24 или другие секреты;
- пароль или содержимое `/root/.my.cnf`;
- SQL-текст, значения SQL-запросов, CRM-данные, тела запросов и ответов;
- сырые access log, URL-параметры, cookies и заголовки авторизации;
- удалённые shell-команды или входящие сетевые сервисы.

## English

Pinguva Agent `0.2.10` sends only technical data required for monitoring.

The agent sends:

- server metrics: CPU, memory, disk, network, uptime and ping;
- on Linux: Disk I/O, selected services and configuration-change facts without
  file contents;
- for connected self-hosted Bitrix24: route and `5xx` aggregates, masked
  traffic sources, MySQL thread counters and fixed current-query categories.

The agent does not send:

- configuration-file contents;
- Bitrix24 webhooks or other secrets;
- the password or contents of `/root/.my.cnf`;
- SQL text, SQL values, CRM data, request or response bodies;
- raw access logs, URL parameters, cookies or authorization headers;
- remote shell commands or inbound network services.
