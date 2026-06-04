# Pinguva Agent Source 0.2.4

Этот каталог содержит исходный код агента Pinguva версии `0.2.4`.

Агент:

- не принимает входящие подключения;
- работает только через исходящий HTTPS;
- не выполняет удалённые команды;
- не отправляет содержимое конфигурационных файлов;
- не отправляет Bitrix24 webhook URL;
- не отправляет CRM-данные и тела ответов Bitrix24.

Это можно проверить по исходникам в этом каталоге.

## Локальная настройка Bitrix24

Версия `0.2.4` поддерживает безопасную локальную настройку коробочного Bitrix24.
Webhook вводится только на сервере клиента скрытым вводом и сохраняется локально
в конфиге агента с ограниченными правами доступа.

Это не отдельный агент. Используется тот же Linux-агент Pinguva, который уже
установлен и подключен к Pinguva на сервере Bitrix24.

Упрощённая команда из UI Pinguva:

```bash
curl -fsSL "https://monit.pinguva.com/install/bitrix24.sh" | sudo bash -s -- --base-url "https://crm.example.kz"
```

Ручная команда без install-сценария:

```bash
sudo pinguva-agent bitrix24 connect \
  --base-url "https://crm.example.kz" \
  --profiles "basic,scope,crm_deals,crm_leads,crm_contacts,crm_statuses"
```

После запуска агент спросит входящий webhook Bitrix24 скрытым вводом. Это не
адрес портала из UI, а секретный webhook, который пользователь создаёт внутри
Bitrix24:

```text
Bitrix24 webhook URL / Входящий webhook Bitrix24 (hidden input, not the portal URL):
```

Webhook URL не попадает в shell history, если его вводить интерактивно.

## REST-профили Bitrix24

Профили — это готовые безопасные проверки. Агент выполняет только исходящие
HTTPS-запросы к Bitrix24 REST и отправляет в Pinguva короткую техническую
сводку.

Доступные профили:

- `basic` — базовая доступность REST через `user.current`; всегда рекомендуется;
- `scope` — проверка прав webhook через `scope`;
- `crm_deals` — техническая проверка списка сделок через `crm.item.list`;
- `crm_leads` — техническая проверка списка лидов через `crm.item.list`;
- `crm_contacts` — техническая проверка списка контактов через `crm.item.list`;
- `crm_statuses` — техническая проверка стадий через `crm.status.list`;
- `method_discovery` — проверка доступности REST-методов через `method.get`,
  включайте вручную только если это действительно нужно.

По умолчанию используются:

```text
basic,scope,crm_deals,crm_leads,crm_contacts,crm_statuses
```

## Что отправляется в Pinguva по Bitrix24

Только summary:

- настроена ли локальная интеграция;
- общий статус проверки;
- время ответа;
- время проверки;
- выбранные профили;
- название REST-метода;
- статус метода;
- задержка метода;
- безопасный count, если метод вернул агрегированное количество;
- короткая ошибка после удаления секретов.

## Что не отправляется

- Bitrix24 webhook URL;
- OAuth token, access token, refresh token;
- тела HTTP-запросов и ответов;
- CRM-записи;
- названия сделок;
- телефоны;
- email;
- комментарии;
- пользовательские поля;
- файлы и вложения.

## Локальная проверка статуса

```bash
sudo pinguva-agent bitrix24 status
```

Команда проверяет локальный конфиг и выполняет короткую REST-проверку без
передачи webhook URL в Pinguva.

## Если агент пишет про `127.0.0.1:8080`

Это означает, что на сервере не задан публичный адрес Pinguva для агента.
Проверьте файл окружения:

```bash
sudo cat /etc/pinguva-agent.env
```

В нём должна быть строка:

```bash
AGENT_SERVER=https://your-pinguva-host
```

После исправления перезапустите агент:

```bash
sudo systemctl restart pinguva-agent
sudo systemctl status pinguva-agent --no-pager
```

## Где взять Bitrix24 webhook

Подробная инструкция по созданию входящего вебхука Bitrix24 и безопасному вводу
в агент:

- `../docs/ru/BITRIX24_LOCAL_INTEGRATION.md`
- `../docs/en/BITRIX24_LOCAL_INTEGRATION.md`
