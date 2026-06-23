# Pinguva Agent Source Archive

English below.

## Русский

Этот каталог хранит публикуемые исходники агента Pinguva по версиям.

Pinguva как платформа мониторинга, её backend, SaaS-сервис и внутренняя серверная логика не входят в этот архив и остаются закрытой частью продукта.
Публикуемый здесь код нужен только для прозрачности и аудита агента.

Что важно для review:
- агент не принимает входящие подключения;
- работает только через исходящий HTTPS;
- не выполняет удалённые команды;
- не отправляет содержимое конфигурационных файлов;
- это можно проверить по исходникам в каталогах версий.

Структура:
- `0.2.3/` — исходный код агента версии 0.2.3
- `0.2.4/` — исходный код агента версии 0.2.4 с локальной настройкой Bitrix24 и REST summary-профилями
- `0.2.5/` — исходный код агента версии 0.2.5 с исправленной проверкой доступности метода Bitrix24
- `0.2.6/` — исходный код агента версии 0.2.6 с локальной сводкой нагрузки Bitrix24 из access log и MySQL без передачи сырых данных
- `0.2.7/` — исходный код агента версии 0.2.7 с безопасным обновлением существующей локальной интеграции Bitrix24 и выбором важных маршрутов
- `0.2.8/` — исходный код агента версии 0.2.8 с явным безопасным использованием локального `/root/.my.cnf` для диагностики MySQL
- `0.2.9/` — исходный код агента версии 0.2.9 с совместимой диагностикой MySQL/MariaDB и независимой обработкой статуса и активных запросов
- `0.2.10/` — исходный код агента версии 0.2.10 с корректным распознаванием глобального `ALL PRIVILEGES` для диагностики MySQL
- `docs/` — публичная документация по агенту и безопасным интеграциям
- `examples/` — примеры collector-кода для разработчиков клиентов

Каждая версия самодостаточна и содержит:
- исходники Linux/Windows агента;
- свой `go.mod`;
- `README.md`;
- `LICENSE`;
- `SECURITY.md`;
- `PRIVACY.md`;
- `CHANGELOG.md`.

Каталог предназначен для ручной публикации отдельных версий в открытых репозиториях.
Версионирование ведётся по релизам агента, как уже начато с `0.2.3`.

Полезные публичные материалы:
- `docs/ru/AGENT_TROUBLESHOOTING_RUNBOOK.md` — диагностика проблем с агентом на стороне клиента
- `docs/ru/API_COLLECTOR_INTEGRATION.md` — подключение сборщика API без передачи персональных данных
- `docs/ru/BITRIX24_LOCAL_INTEGRATION.md` — создание входящего вебхука Bitrix24 и локальная настройка агента
- `docs/en/AGENT_TROUBLESHOOTING_RUNBOOK.md` — English troubleshooting runbook
- `docs/en/API_COLLECTOR_INTEGRATION.md` — English API Collector guide
- `docs/en/BITRIX24_LOCAL_INTEGRATION.md` — English Bitrix24 local integration guide
- `examples/business-api-collector/` — минимальные примеры для Node.js, PHP и FastAPI

Старые ссылки `docs/AGENT_TROUBLESHOOTING_RUNBOOK.md` и
`docs/API_COLLECTOR_INTEGRATION.md` сохранены как страницы выбора языка.

## English

This directory stores public Pinguva agent source snapshots by release version.

Pinguva as a monitoring platform, its backend, SaaS service and internal server
logic are not part of this archive and remain closed-source. The code published
here is provided for transparency and agent security review.

Important review points:

- the agent does not accept inbound connections;
- the agent works through outbound HTTPS only;
- the agent does not execute remote commands;
- the agent does not send configuration file contents;
- this can be verified in the versioned source directories.

Structure:

- `0.2.3/` — agent source code for version 0.2.3
- `0.2.4/` — agent source code for version 0.2.4 with local Bitrix24 checks
- `0.2.5/` — agent source code for version 0.2.5 with a fixed Bitrix24 method-availability check
- `0.2.6/` — agent source code for version 0.2.6 with a local Bitrix24 load summary from access logs and MySQL without raw-data transfer
- `0.2.7/` — agent source code for version 0.2.7 with safe upgrades of existing local Bitrix24 integrations and important-route selection
- `0.2.8/` — agent source code for version 0.2.8 with explicit safe use of local `/root/.my.cnf` for MySQL diagnostics
- `0.2.9/` — agent source code for version 0.2.9 with compatible MySQL/MariaDB diagnostics and independent status/process-list handling
- `0.2.10/` — agent source code for version 0.2.10 with correct global `ALL PRIVILEGES` handling for MySQL diagnostics
- `docs/ru/` — Russian documentation
- `docs/en/` — English documentation
- `examples/` — API Collector examples for customer developers

Useful public materials:

- `docs/en/AGENT_TROUBLESHOOTING_RUNBOOK.md` — troubleshooting runbook
- `docs/en/API_COLLECTOR_INTEGRATION.md` — API Collector integration guide
- `docs/en/BITRIX24_LOCAL_INTEGRATION.md` — Bitrix24 local integration guide
- `docs/ru/AGENT_TROUBLESHOOTING_RUNBOOK.md` — Russian troubleshooting runbook
- `docs/ru/API_COLLECTOR_INTEGRATION.md` — Russian API Collector guide
- `docs/ru/BITRIX24_LOCAL_INTEGRATION.md` — Russian Bitrix24 local integration guide
- `examples/business-api-collector/` — minimal examples for Node.js, PHP and FastAPI
