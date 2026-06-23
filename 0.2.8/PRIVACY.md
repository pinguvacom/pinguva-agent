# Privacy Notes

## Русский

Агент отправляет только технические данные мониторинга: CPU, память, диск,
сеть, uptime, ping, выбранные сервисы и безопасную сводку Bitrix24 REST.

Для Bitrix24 `0.2.8` также передаются агрегаты локальной диагностики:
количество динамических запросов, `5xx`, нормализованные пути без query string,
маскированные источники, счётчики MySQL и фиксированные категории запросов.
Выбранные пользователем пути передаются только как нормализованные маршруты.

Если локальные данные MySQL настроены в `/root/.my.cnf`, этот файл читается
только на сервере клиента. Его содержимое не передаётся в Pinguva и не попадает
в аргументы команд, отчёты агента или журналы.

Не отправляются содержимое файлов, сырые access log, URL-параметры, cookies,
`Authorization`, webhook URL, токены, CRM-данные, HTTP-тела, SQL-текст, UUID и
значения SQL-запросов.

## English

The agent sends only technical monitoring data: CPU, memory, disk, network,
uptime, ping, selected services and a safe Bitrix24 REST summary.

For Bitrix24 `0.2.8`, it also sends local diagnostic aggregates: dynamic request
count, `5xx`, normalized paths without query strings, masked sources, MySQL
counters and fixed query categories. User-selected routes are sent only as
normalized paths.

When local MySQL credentials are configured in `/root/.my.cnf`, that file is
read only on the customer server. Its contents are never sent to Pinguva or
written to command arguments, agent reports or logs.

It does not send file contents, raw access logs, URL parameters, cookies,
`Authorization`, webhook URLs, tokens, CRM data, HTTP bodies, SQL text, UUIDs
or SQL values.
