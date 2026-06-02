# Changelog

## 0.2.3

Что входит в эту версию агента:
- базовая телеметрия сервера: CPU, память, диск, сеть, uptime, ping;
- Linux Disk I/O метрики:
  - read/write throughput;
  - read/write IOPS;
  - disk busy %;
  - CPU iowait %;
- Linux watched services monitoring;
- Linux configuration change control profiles;
- поддержка summary по изменениям конфигурации без отправки содержимого файлов;
- Windows basic host telemetry.

Что важно:
- агент работает только по исходящему HTTPS;
- входящие подключения не требуются;
- удалённые команды не выполняются;
- содержимое конфигурационных файлов не отправляется.
