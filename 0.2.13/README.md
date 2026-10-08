# Pinguva Agent 0.2.13

## Русский

Этот каталог содержит полный исходный код Linux- и Windows-агента Pinguva
версии `0.2.13` для прозрачности и аудита безопасности.

Агент не принимает входящие подключения, работает через исходящее HTTPS и не
выполняет удалённые команды.

Версия `0.2.13` добавляет отдельную диагностику MySQL, MariaDB и PostgreSQL для
обычных Linux-серверов:

- сбор запускается локальным systemd timer раз в минуту;
- используется только один экземпляр СУБД на агент;
- на сервере с настроенным Bitrix24 второй сборщик не запускается;
- MySQL и MariaDB используют безопасный `/root/.my.cnf` или локальный socket;
- PostgreSQL использует фиксированные read-only запросы к системным
  представлениям;
- локальная очередь ограничена 24 часами и 100 MiB;
- передаются только числовые агрегаты, hash и тип технической SQL-группы.

Агент не передаёт пароли, файлы учётных данных, исходный SQL, параметры SQL,
значения таблиц, grants или конфигурационные файлы.

[Инструкция по диагностике СУБД](../docs/ru/DATABASE_DIAGNOSTICS.md)

## English

This directory contains the complete source code for Pinguva Linux and Windows
Agent version `0.2.13`, published for transparency and security review.

The agent accepts no inbound connections, communicates through outbound HTTPS
and does not execute remote commands.

Version `0.2.13` adds separate MySQL, MariaDB and PostgreSQL diagnostics for
regular Linux servers:

- a local systemd timer runs collection once per minute;
- one database instance is supported per agent;
- a second collector is not started when Bitrix24 is configured;
- MySQL and MariaDB use a safe `/root/.my.cnf` or local socket;
- PostgreSQL uses fixed read-only system-view queries;
- the local queue is limited to 24 hours and 100 MiB;
- only numeric aggregates, group hashes and operation types are transmitted.

The agent does not transmit passwords, credential files, source SQL, SQL
parameters, table values, grants or configuration files.

[Database diagnostics guide](../docs/en/DATABASE_DIAGNOSTICS.md)
