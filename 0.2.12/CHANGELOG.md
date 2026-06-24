# Pinguva Agent 0.2.12 Changes

## Русский

- Добавлена минутная история нагрузки коробочного Bitrix24: REST-запросы,
  `5xx`, технические показатели MySQL, SQL digest и инциденты нагрузки.
- Неотправленная историческая очередь хранится только локально с правами
  `root:root`, `0700` для каталога и `0600` для файлов; предел - `24` часа или
  `100 MiB`.
- Дополнительный сбор начинается только после безопасного capability-флага от
  совместимого сервера. Старый backend не запускает этот сбор и не затрагивает
  основную телеметрию.
- Дополнительные агрегаты отправляются отдельным ограниченным endpoint от
  root-owned диагностики. Обычный сервис агента не получает доступ к закрытому
  буферу, webhook или MySQL-конфигурации.
- Добавлено чтение `performance_schema.data_lock_waits`, когда оно поддержано
  MySQL/MariaDB; отсутствие этой таблицы не ухудшает остальные показатели.
- Нормализованный `SELECT` передаётся только при безопасном удалении значений;
  исходный SQL, параметры и `PROCESSLIST.INFO` не передаются.
- Существующие webhook, REST-профили и пользовательские access-log пути не
  меняются при обновлении.

## English

- Added minute-level self-hosted Bitrix24 load history: REST requests, `5xx`,
  technical MySQL metrics, SQL digests and load incidents.
- Unsent historical queue storage is local only with `root:root`, a `0700`
  directory and `0600` files; it is limited to `24` hours or `100 MiB`.
- Extra collection begins only after a safe capability flag from a compatible
  server. An older backend does not start this collection or affect baseline
  telemetry.
- Optional aggregates are sent through a separate bounded endpoint by the
  root-owned diagnostics task. The normal agent service does not gain access to
  the closed buffer, webhook or MySQL configuration.
- Added `performance_schema.data_lock_waits` collection when supported by
  MySQL/MariaDB; unsupported lock data does not downgrade other metrics.
- A normalized `SELECT` is sent only when values are safely removed; source SQL,
  parameters and `PROCESSLIST.INFO` are never sent.
- Existing webhooks, REST profiles and custom access-log paths remain unchanged
  during an update.
