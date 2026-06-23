# Privacy Notes

## Русский

Агент отправляет в Pinguva только технические данные, необходимые для
мониторинга: CPU, память, диск, сеть, uptime, ping, выбранные сервисы и
безопасную сводку Bitrix24 REST.

Для локальной диагностики Bitrix24 `0.2.6` также отправляются агрегаты:
количество динамических запросов, `5xx`, нормализованные пути без query string,
маскированные источники, счётчики MySQL и фиксированные категории запросов.

Не отправляются содержимое файлов, сырые access log, URL-параметры, cookies,
`Authorization`, webhook URL, токены, CRM-данные, HTTP-тела, SQL-текст, UUID и
значения SQL-запросов.

## English

The agent sends only technical monitoring data to Pinguva: CPU, memory, disk,
network, uptime, ping, selected services and a safe Bitrix24 REST summary.

For Bitrix24 local diagnostics in `0.2.6`, it also sends aggregates: dynamic
request count, `5xx`, normalized paths without query strings, masked sources,
MySQL counters and fixed query categories.

It does not send file contents, raw access logs, URL parameters, cookies,
`Authorization`, webhook URLs, tokens, CRM data, HTTP bodies, SQL text, UUIDs
or SQL values.
