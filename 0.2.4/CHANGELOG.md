# Pinguva Agent 0.2.4

## Что изменилось

- Агент теперь читает стандартный `/etc/pinguva-agent.env` при ручном запуске
  и не уходит молча на `http://127.0.0.1:8080`, если `AGENT_SERVER` в env-файле
  пустой.
- Добавлена локальная команда `pinguva-agent bitrix24 configure` для настройки
  входящего webhook Bitrix24 через скрытый ввод.
- Добавлен выбор безопасных Bitrix24 REST-профилей через `--profiles`.
- Агент выполняет исходящие HTTPS-проверки выбранных REST-методов Bitrix24.
- В Pinguva отправляется только summary: статус, задержка, безопасный count и
  короткая ошибка после удаления секретов.
- Добавлена локальная команда `pinguva-agent bitrix24 status` для проверки
  конфигурации на сервере клиента.

## REST-профили

- `basic` — `user.current`;
- `scope` — `scope`;
- `crm_deals` — `crm.item.list` для сделок;
- `crm_leads` — `crm.item.list` для лидов;
- `crm_contacts` — `crm.item.list` для контактов;
- `crm_statuses` — `crm.status.list` для стадий;
- `method_discovery` — `method.get`, вручную при необходимости.

## Безопасность

- Webhook URL Bitrix24 хранится только локально на сервере клиента.
- Webhook URL не передается в Pinguva и не показывается в веб-интерфейсе.
- Тела ответов Bitrix24, CRM-данные и пользовательские данные не отправляются.
- Ошибки перед отправкой проходят redaction: webhook secret, auth/access/refresh
  token и похожие параметры заменяются на `[redacted]`.
