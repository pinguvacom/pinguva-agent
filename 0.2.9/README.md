# Pinguva Agent 0.2.9

## Русский

Этот каталог содержит исходный код Linux- и Windows-агента Pinguva версии
`0.2.9`.

Агент:

- не принимает входящие сетевые подключения;
- работает только через исходящее HTTPS-соединение к Pinguva;
- не выполняет удалённые команды;
- не отправляет содержимое конфигурационных файлов;
- не передаёт webhook Bitrix24, пароли MySQL, SQL-текст, значения запросов,
  сырые строки access log или CRM-данные.

В этой версии улучшена локальная диагностика MySQL/MariaDB для коробочного
Bitrix24:

- основной источник потоков - `performance_schema.global_status`;
- резервные источники - `SHOW GLOBAL STATUS` и
  `information_schema.GLOBAL_STATUS`;
- потоки и текущие запросы собираются независимо;
- `/root/.my.cnf` используется только как обычный файл `root:root` с правами
  `0600`; при несоответствии агент переходит на локальный socket и не меняет
  файл.

Полная пользовательская инструкция: [Bitrix24 local integration](../docs/ru/BITRIX24_LOCAL_INTEGRATION.md).

## English

This directory contains the source code of Pinguva Linux and Windows Agent
version `0.2.9`.

The agent:

- does not accept inbound network connections;
- works only through an outbound HTTPS connection to Pinguva;
- does not execute remote commands;
- does not send configuration-file contents;
- does not send Bitrix24 webhooks, MySQL passwords, SQL text, query values,
  raw access-log lines, or CRM data.

This release improves local MySQL/MariaDB diagnostics for self-hosted Bitrix24:

- primary thread-status source: `performance_schema.global_status`;
- fallbacks: `SHOW GLOBAL STATUS` and
  `information_schema.GLOBAL_STATUS`;
- thread status and current-query collection are independent;
- `/root/.my.cnf` is used only as a regular `root:root` file with `0600`
  permissions; otherwise the agent falls back to the local socket without
  modifying the file.

Full user documentation: [Bitrix24 local integration](../docs/en/BITRIX24_LOCAL_INTEGRATION.md).
