# Privacy Notes

## Русский

Pinguva Agent `0.2.12` передаёт только технические данные, необходимые для
мониторинга.

Агент передаёт:

- системные метрики сервера: CPU, память, диск, сеть, uptime и ping;
- для Linux: Disk I/O, выбранные сервисы и факт изменения конфигурационных
  файлов без их содержимого;
- для подключенного коробочного Bitrix24: агрегаты маршрутов и `5xx`,
  технические MySQL-счётчики, состояние ожиданий блокировок при поддержке
  сервера и дельты SQL digest;
- только обезличенную структуру `SELECT`, если локальная нормализация смогла
  удалить значения. В противном случае - только digest, категория и счётчики.

Агент не передаёт:

- содержимое конфигурационных файлов;
- webhook Bitrix24, пароль или содержимое `/root/.my.cnf`;
- CRM-данные, тела запросов и ответов, сырые access log;
- исходный SQL, значения SQL-запросов, `PROCESSLIST.INFO`, grants;
- URL-параметры, cookies, заголовки авторизации;
- удалённые shell-команды, входящие сетевые сервисы или произвольные файлы.

## English

Pinguva Agent `0.2.12` sends only technical data needed for monitoring.

The agent sends:

- server metrics: CPU, memory, disk, network, uptime and ping;
- on Linux: Disk I/O, selected services and configuration-change facts without
  file contents;
- for connected self-hosted Bitrix24: route and `5xx` aggregates, technical
  MySQL counters, compatible lock-wait status and SQL digest deltas;
- a redacted `SELECT` structure only when local normalization has removed values.
  Otherwise it sends only digest, category and counters.

The agent does not send:

- configuration-file contents;
- Bitrix24 webhooks, the password or contents of `/root/.my.cnf`;
- CRM data, HTTP request or response bodies, raw access logs;
- source SQL, SQL values, `PROCESSLIST.INFO` or grants;
- URL parameters, cookies, authorization headers, remote shell commands,
  inbound network services or arbitrary server files.
