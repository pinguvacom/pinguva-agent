# Pinguva Agent 0.2.10 Changes

## Русский

- Исправлено определение права `PROCESS` для MySQL: глобальное
  `ALL PRIVILEGES ON *.*` теперь корректно считается достаточным для чтения
  текущих запросов.
- Исправление не меняет выдачу прав, подключение к MySQL или данные, которые
  передаются в Pinguva. Оно убирает ложный статус `restricted` у root-подключений
  с глобальными правами.

## English

- Fixed MySQL `PROCESS` privilege detection: a global
  `ALL PRIVILEGES ON *.*` grant is now correctly treated as sufficient to read
  current queries.
- This does not change MySQL grants, connection behaviour or data sent to
  Pinguva. It removes a false `restricted` state for root connections with
  global privileges.
