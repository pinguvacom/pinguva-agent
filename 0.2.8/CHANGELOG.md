# Pinguva Agent 0.2.8 Changes

## Русский

- Локальная диагностика MySQL теперь явно использует `/root/.my.cnf`, если файл
  существует, является обычным файлом и не доступен для записи группе или
  остальным пользователям.
- Пароль MySQL остаётся в локальном файле: он не передаётся в аргументах,
  переменных окружения, отчётах агента или журналах Pinguva.
- При отсутствии подходящего `/root/.my.cnf` агент сохраняет прежнюю попытку
  подключения через стандартный локальный socket MySQL.
- Добавлена понятная справка в карточке сервера, когда недоступны только
  показатели MySQL, а сводка трафика и REST-проверки продолжают работать.

## English

- Local MySQL diagnostics now explicitly use `/root/.my.cnf` when it exists,
  is a regular file and is not writable by group or other users.
- The MySQL password remains in the local file: it is never sent through
  command arguments, environment variables, agent reports or Pinguva logs.
- When there is no suitable `/root/.my.cnf`, the agent keeps the existing
  connection attempt through the standard local MySQL socket.
- Added clear server-card help when only MySQL metrics are unavailable while
  traffic summary and REST checks continue to work.
