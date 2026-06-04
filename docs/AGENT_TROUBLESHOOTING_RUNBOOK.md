# Pinguva Agent Troubleshooting Runbook

Пошаговый сценарий диагностики проблем с агентами Pinguva.

Документ нужен для типовых случаев:

- агент отображается `offline`;
- агент не появляется после установки;
- метрики перестали обновляться;
- обновление агента прошло, но телеметрия не возвращается;
- один конкретный сервер не может достучаться до `monit.pinguva.com`.

## Базовые факты

- агент по умолчанию отправляет телеметрию раз в `60` секунд;
- offline для агента по умолчанию считается после `120` секунд без heartbeat;
- агенту не нужны входящие порты;
- агенту нужен исходящий `HTTPS`-доступ к `monit.pinguva.com:443`;
- основной endpoint приёма телеметрии: `POST /api/agent/report`;
- endpoint первичной привязки: `POST /api/agent/enroll`.

## Где лежат логи и конфиг

### Linux-агент

- сервис: `pinguva-agent`
- unit: `/etc/systemd/system/pinguva-agent.service`
- env: `/etc/pinguva-agent.env`
- state: `/var/lib/pinguva-agent/state.json`
- рабочий каталог: `/var/lib/pinguva-agent`
- бинарник: `/usr/bin/pinguva-agent`
- логи по умолчанию: `journald`

Основные команды:

```bash
systemctl status pinguva-agent
journalctl -u pinguva-agent -n 200 --no-pager
journalctl -u pinguva-agent -f
systemctl cat pinguva-agent
cat /etc/pinguva-agent.env
cat /var/lib/pinguva-agent/state.json
```

### Windows-агент

- сервис: `PinguvaAgent`
- бинарник: `C:\Program Files\Pinguva Agent\pinguva-agent.exe`
- state: `C:\ProgramData\Pinguva\agent\state.json`
- tray: `C:\Program Files\Pinguva Agent\pinguva-agent-tray.exe`

Важно:

- отдельный лог-файл по умолчанию не создаётся;
- для проблем старта и остановки смотри `Service Control Manager` в Windows Event Viewer;
- для детальной диагностики лучше временно запускать агент вручную в PowerShell и смотреть stdout.

Основные команды:

```powershell
Get-Service PinguvaAgent
sc.exe query PinguvaAgent
sc.exe qc PinguvaAgent
Get-Content C:\ProgramData\Pinguva\agent\state.json
Get-WinEvent -LogName System -MaxEvents 200 | Where-Object {$_.ProviderName -eq "Service Control Manager"}
```

## Быстрая классификация ошибок

### `context deadline exceeded (Client.Timeout exceeded while awaiting headers)`

Обычно это один из двух сценариев:

- агент не смог получить HTTP-ответ за `15` секунд;
- соединение до `443` прошло, но reverse proxy или backend завис до отправки headers.

Что проверять:

- доступность `TCP/443` до `monit.pinguva.com`;
- время ответа `curl` с проблемного сервера;
- access log и error log reverse proxy;
- логи сервиса `pinguva`;
- состояние backend и базы, если запросы доходят до приложения.

### `Failed to connect ... port 443: Timeout was reached`

Это уже сетевой путь, не токен и не JSON.

Обычно причина в одном из пунктов:

- firewall на клиенте;
- firewall на стороне сервера;
- маршрут между сетями;
- фильтрация у хостера;
- proxy или egress policy;
- broken DNS, если клиент ходит не туда.

### `no such host`, `lookup ...`

Это DNS-проблема.

Проверять:

- `dig`;
- `resolvectl`;
- `/etc/resolv.conf`;
- локальный DNS-cache;
- доступность DNS-серверов.

### `401 Unauthorized` или `403 Forbidden`

Сеть и приложение отвечают, но проблема в токене или привязке.

Проверять:

- актуальность agent token;
- содержимое `state.json`;
- не был ли агент перепривязан;
- есть ли в env корректный enrollment token для автоматического re-enroll.

### `400 Bad Request`, `невалидный JSON`

Сеть и токен работают. Проблема уже в клиентском payload или несовместимой версии агента.

## Минимальный чек-лист на стороне клиента Linux

### 1. Проверить состояние сервиса

```bash
systemctl status pinguva-agent
journalctl -u pinguva-agent -n 100 --no-pager
```

Смотреть:

- сервис `active (running)` или нет;
- повторяющиеся `report failed`;
- `enrollment failed`;
- `report auth failed`;
- `no AGENT_TOKEN and no AGENT_ENROLLMENT_TOKEN`.

### 2. Проверить конфиг агента

```bash
cat /etc/pinguva-agent.env
cat /var/lib/pinguva-agent/state.json
```

Проверить:

- `AGENT_SERVER` указывает на правильный адрес;
- `AGENT_NAME` ожидаемый;
- `AGENT_STATE_PATH` указывает на реальный файл;
- state-файл существует и не пустой;
- agent ID и token выглядят валидно.

Важно:

- `AGENT_ENROLLMENT_TOKEN` нужен для первичной привязки и auto re-enroll;
- после нормальной привязки рабочий agent token хранится в `state.json`;
- если токен уже в state-файле невалиден, но enrollment token остался, агент сможет перепривязаться автоматически.

### 3. Проверить базовую HTTPS-связность до `monit`

```bash
time curl -vk --connect-timeout 5 --max-time 20 https://monit.pinguva.com/
time curl -vk --connect-timeout 5 --max-time 20 https://monit.pinguva.com/api/agent/report
time curl -vk --connect-timeout 5 --max-time 20 -X POST https://monit.pinguva.com/api/agent/report -H 'Content-Type: application/json' -d '{}'
```

Как интерпретировать:

- быстрый `200`, `301`, `400`, `401`, `405` означает, что сеть до сервера живая;
- `connect timeout` означает, что до `443` нет маршрута или трафик режется;
- `awaiting headers` означает, что connection или TLS могли пройти, но приложение не ответило вовремя.

### 4. Проверить DNS

```bash
getent hosts monit.pinguva.com
dig +short monit.pinguva.com
resolvectl query monit.pinguva.com
cat /etc/resolv.conf
```

Если есть сомнения в DNS:

```bash
curl -vk --resolve monit.pinguva.com:443:<PINGUVA_SERVER_IP> https://monit.pinguva.com/
```

Это помогает отделить проблему DNS от проблемы TCP-маршрута.

### 5. Проверить TCP-маршрут и firewall

```bash
nc -vz -w5 monit.pinguva.com 443
openssl s_client -connect monit.pinguva.com:443 -servername monit.pinguva.com -brief
ip route get <PINGUVA_SERVER_IP>
tracepath <PINGUVA_SERVER_IP>
mtr -T -P 443 -rw <PINGUVA_SERVER_IP>
sudo nft list ruleset
sudo iptables -S OUTPUT
ufw status verbose
```

Как читать:

- `nc` timeout: проблема ещё до HTTP;
- `openssl` не соединяется: проблема в TCP-маршруте или firewall;
- `openssl` соединяется, но `curl` висит: смотреть proxy, TLS, reverse proxy, backend.

### 6. Проверить, что проблема не общая для исходящего `443`

```bash
curl -4vkI --connect-timeout 5 --max-time 10 https://google.com
curl -4vkI --connect-timeout 5 --max-time 10 https://cloudflare.com
```

Если внешние `443` работают, а `monit.pinguva.com:443` нет, проблема точечная:

- firewall на стороне Pinguva;
- upstream-фильтрация;
- блокировка конкретного IP;
- broken route между сетями.

## Минимальный чек-лист на стороне Windows

### 1. Проверить сервис

```powershell
Get-Service PinguvaAgent
sc.exe query PinguvaAgent
sc.exe qc PinguvaAgent
```

### 2. Проверить state

```powershell
Get-Content C:\ProgramData\Pinguva\agent\state.json
```

### 3. Проверить HTTPS и DNS

```powershell
Resolve-DnsName monit.pinguva.com
Test-NetConnection monit.pinguva.com -Port 443
curl.exe -vk --connect-timeout 5 --max-time 20 https://monit.pinguva.com/api/agent/report
```

### 4. Если нужен подробный лог

Остановить сервис и временно запустить агент вручную в elevated PowerShell:

```powershell
Stop-Service PinguvaAgent
& "C:\Program Files\Pinguva Agent\pinguva-agent.exe" -server=https://monit.pinguva.com -state-path="C:\ProgramData\Pinguva\agent\state.json"
```

Если агент был установлен с нестандартными параметрами, сначала посмотри точный `binPath` через:

```powershell
sc.exe qc PinguvaAgent
```

После диагностики вернуть сервис:

```powershell
Start-Service PinguvaAgent
```

## Готовый Linux triage-блок

Ниже минимальный набор команд, который можно запускать по одному сценарию на проблемном Linux-хосте.

```bash
hostname -f
date -Is
systemctl status pinguva-agent --no-pager
journalctl -u pinguva-agent -n 100 --no-pager
cat /etc/pinguva-agent.env
cat /var/lib/pinguva-agent/state.json
getent hosts monit.pinguva.com
dig +short monit.pinguva.com
nc -vz -w5 monit.pinguva.com 443
curl -vk --connect-timeout 5 --max-time 20 https://monit.pinguva.com/api/agent/report
ip route get <PINGUVA_SERVER_IP>
tracepath <PINGUVA_SERVER_IP>
sudo nft list ruleset
sudo iptables -S OUTPUT
```

## Что не делать без явной причины

- не удалять `state.json`, если задача не в перепривязке агента;
- не переводить `AGENT_SERVER` на raw IP как постоянное решение;
- не отключать TLS-проверку;
- не перезапускать агент циклически без чтения логов;
- не считать `ping` единственным тестом доступности `443`.

## Практическая интерпретация типовых ситуаций

### Сервер жив по SSH, а агент `offline`

Это нормально с точки зрения симптомов. Значит:

- сам хост доступен;
- но агентский процесс не работает, не может резолвить DNS, не может выйти по `443` или не получает ответ от Pinguva.

### DNS имя резолвится, но `curl` до `443` висит на connect timeout

Это уже не DNS. Проверять:

- маршрут;
- firewall;
- egress policy;
- блокировку на стороне сервера;
- хостера или провайдера.

### `curl` быстро отвечает `401` или `405`, но агент всё ещё не работает

Тогда сеть до `monit` живая. Смотреть:

- токен;
- `state.json`;
- версию агента;
- логи самого сервиса;
- логи backend по `/api/agent/report`.

## Когда эскалировать на сторону Pinguva

Эскалировать на серверную сторону нужно, если выполняется хотя бы одно из условий:

- несколько агентов из разных сетей одновременно перестали достукиваться;
- с проблемного хоста `TCP/443` доходит, но агент и `curl` ждут headers слишком долго;
- reverse proxy видит запросы, но клиент ловит timeout;
- backend пишет ошибки обработки agent report;
- есть подозрение на блокировку IP на стороне `monit`.
