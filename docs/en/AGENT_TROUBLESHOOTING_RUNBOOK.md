# Pinguva Agent Troubleshooting Runbook

Use this runbook when:

- the agent is shown as offline;
- the agent does not appear after installation;
- telemetry stopped updating;
- an update completed but telemetry did not return;
- one server cannot reach `monit.pinguva.com`.

## Basic Facts

- The agent sends telemetry every `60` seconds by default.
- The default offline timeout is `120` seconds without telemetry.
- The agent does not need inbound ports.
- The agent needs outbound HTTPS access to `monit.pinguva.com:443`.
- Telemetry endpoint: `POST /api/agent/report`.
- First enrollment endpoint: `POST /api/agent/enroll`.

## Linux Paths

- service: `pinguva-agent`
- unit: `/etc/systemd/system/pinguva-agent.service`
- environment file: `/etc/pinguva-agent.env`
- state file: `/var/lib/pinguva-agent/state.json`
- working directory: `/var/lib/pinguva-agent`
- binary: `/usr/bin/pinguva-agent`
- logs: `journald`

Useful commands:

```bash
systemctl status pinguva-agent
journalctl -u pinguva-agent -n 200 --no-pager
journalctl -u pinguva-agent -f
systemctl cat pinguva-agent
cat /etc/pinguva-agent.env
cat /var/lib/pinguva-agent/state.json
```

## Windows Paths

- service: `PinguvaAgent`
- binary: `C:\Program Files\Pinguva Agent\pinguva-agent.exe`
- state file: `C:\ProgramData\Pinguva\agent\state.json`
- tray app: `C:\Program Files\Pinguva Agent\pinguva-agent-tray.exe`

Useful commands:

```powershell
Get-Service PinguvaAgent
sc.exe query PinguvaAgent
sc.exe qc PinguvaAgent
Get-Content C:\ProgramData\Pinguva\agent\state.json
Get-WinEvent -LogName System -MaxEvents 200 | Where-Object {$_.ProviderName -eq "Service Control Manager"}
```

## Common Errors

### `context deadline exceeded`

Usually the agent could not receive an HTTP response within `15` seconds.

Check:

- outbound `TCP/443` access;
- response time with `curl`;
- reverse proxy logs;
- Pinguva service logs;
- database and application health if requests reach the platform.

### `Failed to connect ... port 443`

This is a network path issue, not a token issue.

Check:

- customer firewall;
- server firewall;
- route between networks;
- hosting provider filtering;
- proxy or egress policy;
- DNS resolution.

### `no such host` or `lookup ...`

This is a DNS issue.

Check:

```bash
dig monit.pinguva.com
resolvectl status
cat /etc/resolv.conf
```

### `401 Unauthorized` or `403 Forbidden`

Network and application are reachable, but token or enrollment is wrong.

Check:

- current agent token;
- `state.json`;
- whether the agent was re-enrolled;
- whether `AGENT_ENROLLMENT_TOKEN` is still available for automatic re-enrollment.

### Reports go to `127.0.0.1:8080`

The agent does not have the public Pinguva URL configured.

Check:

```bash
sudo cat /etc/pinguva-agent.env
```

It must contain:

```bash
AGENT_SERVER=https://monit.pinguva.com
```

After fixing it:

```bash
sudo systemctl restart pinguva-agent
sudo systemctl status pinguva-agent --no-pager
```

## Minimal Linux Checklist

```bash
systemctl status pinguva-agent
journalctl -u pinguva-agent -n 100 --no-pager
cat /etc/pinguva-agent.env
cat /var/lib/pinguva-agent/state.json
curl -I https://monit.pinguva.com/
```

Verify:

- service is `active (running)`;
- `AGENT_SERVER` points to the correct public Pinguva URL;
- `AGENT_STATE_PATH` points to the real state file;
- state file has an agent ID or the env file has an enrollment token;
- outbound HTTPS works.

## Bitrix24 Integration Checklist

Bitrix24 REST checks require Linux agent `0.2.5` or newer. Local load
diagnostics require `0.2.6` or newer.

```bash
sudo pinguva-agent bitrix24 status
sudo systemctl status pinguva-bitrix24-diagnostics.timer --no-pager
sudo journalctl -u pinguva-bitrix24-diagnostics.service -n 50 --no-pager
```

If the command is unknown, update the agent binary first.

```bash
curl -fsSL https://monit.pinguva.com/install/pinguva-agent-linux-amd64 -o /tmp/pinguva-agent
sudo install -m 0755 /tmp/pinguva-agent /usr/bin/pinguva-agent
sudo systemctl restart pinguva-agent
```
