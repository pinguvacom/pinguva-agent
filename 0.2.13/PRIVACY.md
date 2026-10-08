# Privacy Notes

## Русский

Pinguva Agent `0.2.13` передаёт системные метрики сервера и только включённые
пользователем дополнительные технические агрегаты.

Для универсальной диагностики СУБД передаются тип и версия СУБД, безопасный
локальный endpoint, числовые показатели, статусы источников, hash и тип
операции технической группы. Исходный SQL не входит в модель события.

Не передаются пароли, `/root/.my.cnf`, `.pgpass`, строки подключения с
секретами, grants, исходный SQL, параметры, значения таблиц и конфигурационные
файлы.

## English

Pinguva Agent `0.2.13` transmits server metrics and only optional technical
aggregates explicitly enabled by the user.

Universal database diagnostics transmit database type and version, a safe
local endpoint, numeric metrics, source statuses, group hashes and operation
types. Source SQL is not part of the event model.

Passwords, `/root/.my.cnf`, `.pgpass`, secret connection strings, grants,
source SQL, parameters, table values and configuration files are not sent.
