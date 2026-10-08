# Pinguva Agent 0.2.13 Changes

## Русский

- Добавлена отдельная универсальная диагностика MySQL, MariaDB и PostgreSQL.
- Добавлены root-owned `pinguva-database-diagnostics.service` и timer с запуском
  раз в минуту и защитой от параллельного выполнения.
- Добавлены безопасные снимки подключений, активных запросов, ожиданий,
  блокировок, накопительных счётчиков и технических SQL-групп.
- Для PostgreSQL накопительные счётчики преобразуются в дельты между снимками.
- Локальная очередь ограничена 24 часами и 100 MiB.
- Настройка блокируется, если на сервере уже работает диагностика Bitrix24.
- Пароли, исходный SQL, параметры и значения таблиц не передаются.

## English

- Added separate universal MySQL, MariaDB and PostgreSQL diagnostics.
- Added root-owned `pinguva-database-diagnostics.service` and timer with
  once-per-minute execution and overlap protection.
- Added safe snapshots for connections, active queries, waits, locks,
  cumulative counters and technical query groups.
- PostgreSQL cumulative counters are converted to deltas between samples.
- The local queue is limited to 24 hours and 100 MiB.
- Setup is blocked when Bitrix24 diagnostics are already configured.
- Passwords, source SQL, parameters and table values are not transmitted.
