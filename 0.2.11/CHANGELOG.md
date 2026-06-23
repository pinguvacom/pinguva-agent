# Pinguva Agent 0.2.11 Changes

## Русский

- Убрано ложное определение отсутствия `PROCESS`: `SHOW GRANTS` теперь
  разбирается как список глобальных привилегий, а не как одна фиксированная
  строка.
- Добавлена функциональная проверка видимости чужих MySQL-сессий без передачи
  имён пользователей, SQL-текста или grants в Pinguva.
- Успешный `PROCESSLIST` больше не блокируется ошибкой или отсутствием
  `SHOW GRANTS`. Пустые активные запросы и пустые группы имеют статус `ok`.
- В безопасной сводке появились статусы `PROCESSLIST`, видимость и источник
  подтверждения права. Миграция базы не нужна: это новые поля существующего
  JSON-отчёта.

## English

- Removed false missing-`PROCESS` detection: `SHOW GRANTS` is now parsed as a
  list of global privileges rather than one fixed string.
- Added a functional check for visibility of other MySQL sessions without
  sending usernames, SQL text or grants to Pinguva.
- A successful `PROCESSLIST` is no longer blocked by an unavailable or
  inconclusive `SHOW GRANTS`. Empty active queries and query groups are `ok`.
- The safe summary now includes `PROCESSLIST` status, visibility and the
  privilege evidence source. No database migration is required because these
  are new fields in the existing JSON report.
