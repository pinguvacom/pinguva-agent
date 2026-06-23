# Pinguva Agent 0.2.9 Changes

## Русский

- Исправлен сбор `Threads_running` и `Threads_connected` на серверах, где нет
  `information_schema.GLOBAL_STATUS`.
- Основной источник метрик - `performance_schema.global_status`; при его
  недоступности агент последовательно использует `SHOW GLOBAL STATUS` и старый
  `information_schema.GLOBAL_STATUS`.
- `PROCESSLIST` и глобальные показатели собираются независимо. Ошибка одного
  источника не удаляет данные другого.
- Пустые значения `active_queries=0` и `longest_query_seconds=0` считаются
  нормальным состоянием, а не ошибкой диагностики.
- Локальный `/root/.my.cnf` принимается только как обычный файл `root:root` с
  правами `0600`. Небезопасный файл не читается и не изменяется.
- В journald добавлены безопасные поля: способ подключения, источник статуса,
  факт fallback и коды недоступных групп метрик. Пароли, содержимое конфигурации
  и тексты SQL не журналируются.

## English

- Fixed collection of `Threads_running` and `Threads_connected` on servers
  without `information_schema.GLOBAL_STATUS`.
- Primary metric source: `performance_schema.global_status`; when unavailable,
  the agent tries `SHOW GLOBAL STATUS` and then the legacy
  `information_schema.GLOBAL_STATUS`.
- `PROCESSLIST` and global metrics are collected independently. A failure in
  one source does not remove data from the other.
- Empty values `active_queries=0` and `longest_query_seconds=0` are normal
  diagnostics, not an error.
- Local `/root/.my.cnf` is accepted only as a regular `root:root` file with
  `0600` permissions. Unsafe files are neither read nor modified.
- journald now contains safe fields for connection mode, status source,
  fallback use and unavailable metric groups. Passwords, configuration
  contents and SQL text are never logged.
