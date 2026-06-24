# Pinguva Agent 0.2.12

## Русский

Этот каталог содержит полный исходный код Linux- и Windows-агента Pinguva
версии `0.2.12` для прозрачности и security review.

Агент:

- не принимает входящие сетевые подключения;
- работает только через исходящее HTTPS-соединение к Pinguva;
- не выполняет удалённые команды;
- не отправляет содержимое конфигурационных файлов;
- не передаёт webhook Bitrix24, пароли MySQL, сырые access log, исходный SQL,
  значения SQL, CRM-данные и тела HTTP-запросов или ответов.

В `0.2.12` добавлена расширенная история нагрузки коробочного Bitrix24:

- root-owned timer формирует безопасные минутные агрегаты REST, MySQL и SQL
  digest;
- неотправленные агрегаты хранятся локально максимум `24` часа или `100 MiB` и
  отправляются отдельным ограниченным endpoint с существующим токеном агента;
- дополнительный сбор включается только после безопасного булевого подтверждения
  от совместимого сервера Pinguva;
- MySQL lock waits собираются только при наличии совместимого
  `performance_schema.data_lock_waits`;
- тяжёлые SQL-группы передают digest, технические счётчики и только
  гарантированно обезличенную структуру `SELECT`; если безопасная нормализация
  невозможна, текст вообще не отправляется;
- webhook, REST-профили и пользовательские access-log пути не меняются при
  обычном обновлении агента.

Полная пользовательская инструкция: [локальная интеграция Bitrix24](../docs/ru/BITRIX24_LOCAL_INTEGRATION.md).

## English

This directory contains the complete source code for Pinguva Linux and Windows
Agent version `0.2.12`, published for transparency and security review.

The agent:

- does not accept inbound network connections;
- works only through an outbound HTTPS connection to Pinguva;
- does not execute remote commands;
- does not send configuration-file contents;
- does not send Bitrix24 webhooks, MySQL passwords, raw access logs, source SQL,
  SQL values, CRM data, or HTTP request and response bodies.

Version `0.2.12` adds extended self-hosted Bitrix24 load history:

- a root-owned timer produces safe minute REST, MySQL and SQL digest aggregates;
- unsent aggregates are stored locally for at most `24` hours or `100 MiB` and
  are sent through a separate bounded endpoint using the existing agent token;
- extra collection starts only after a compatible Pinguva server sends a safe
  boolean capability confirmation;
- MySQL lock waits are collected only when
  `performance_schema.data_lock_waits` is compatible;
- heavy SQL groups send digest and technical counters plus a redacted `SELECT`
  structure only when normalization is proven safe; otherwise no SQL text is sent;
- a normal agent update does not alter the webhook, REST profiles or custom
  access-log paths.

Full user documentation: [Bitrix24 local integration](../docs/en/BITRIX24_LOCAL_INTEGRATION.md).
