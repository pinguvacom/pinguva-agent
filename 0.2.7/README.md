# Pinguva Agent 0.2.7

## Русский

Версия `0.2.7` безопасно обновляет уже настроенную локальную интеграцию
коробочного Bitrix24. Отдельный агент не устанавливается.

При обычном обновлении Linux-агента команда `bitrix24 bootstrap` работает
только если на сервере уже есть `/etc/pinguva-agent/bitrix24.json`. Она не
меняет webhook и REST-профили, добавляет стандартную локальную диагностику и
включает `pinguva-bitrix24-diagnostics.timer`.

Через одну-две минуты карточка сервера показывает агрегаты access log и MySQL.
В Pinguva можно выбрать до десяти уже обнаруженных важных маршрутов. Их список
хранится как настройка Pinguva и передаётся агенту только в ответе на его
обычный исходящий отчёт. Выбранные маршруты остаются в локальной сводке даже
при низком трафике.

Агент не принимает входящие подключения, работает только через исходящий HTTPS,
не выполняет удалённые команды и не отправляет содержимое конфигурационных
файлов. Это можно проверить по исходникам.

Проверка локальной диагностики:

```bash
sudo systemctl status pinguva-bitrix24-diagnostics.timer --no-pager
sudo journalctl -u pinguva-bitrix24-diagnostics.service -n 50 --no-pager
```

Полная инструкция:

- [Русская документация Bitrix24](../docs/ru/BITRIX24_LOCAL_INTEGRATION.md)
- [English Bitrix24 documentation](../docs/en/BITRIX24_LOCAL_INTEGRATION.md)

Проверка исходников:

```bash
go test .
go build .
```

## English

Version `0.2.7` safely upgrades an already configured local self-hosted
Bitrix24 integration. It does not install a separate agent.

During a normal Linux-agent update, `bitrix24 bootstrap` runs only when
`/etc/pinguva-agent/bitrix24.json` already exists. It does not change the
webhook or REST profiles, adds the standard local diagnostics and enables the
`pinguva-bitrix24-diagnostics.timer`.

Within one to two minutes, the server card shows access-log and MySQL
aggregates. Up to ten discovered important routes can be selected in Pinguva.
The selection is stored as Pinguva configuration and reaches the agent only in
the response to its regular outbound report. Selected routes remain in the
local summary when traffic is low.

The agent accepts no inbound connections, uses outbound HTTPS only, does not
execute remote commands and does not send configuration-file contents. This can
be verified in the source code.

Check local diagnostics:

```bash
sudo systemctl status pinguva-bitrix24-diagnostics.timer --no-pager
sudo journalctl -u pinguva-bitrix24-diagnostics.service -n 50 --no-pager
```

Full documentation:

- [Russian Bitrix24 documentation](../docs/ru/BITRIX24_LOCAL_INTEGRATION.md)
- [English Bitrix24 documentation](../docs/en/BITRIX24_LOCAL_INTEGRATION.md)

Verify the source archive:

```bash
go test .
go build .
```
