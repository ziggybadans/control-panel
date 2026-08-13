# Deployment guide

## Requirements

- Debian (or any systemd Linux) on amd64
- Optional integrations, enabled if present:
  - `smartmontools` (`apt install smartmontools`) — disk SMART health
  - `snapraid` — parity status / sync / scrub (set `storage.snapraid.config`)
  - a Java runtime (`apt install openjdk-21-jre-headless`) — Minecraft servers
  - Plex — set `plex.token` in the config
  - qBittorrent — set `qbittorrent.url` (plus WebUI credentials) in the config

## Install

On your workstation (or the server, with Go + Node installed):

```sh
make release        # -> dist/control-panel-linux-amd64
```

Copy the repo (or just `dist/` + `deploy/`) to the server, then:

```sh
sudo ./deploy/install.sh
control-panel hash                       # generate the password hash
sudo nano /etc/control-panel/config.yaml # paste hash, review settings
sudo systemctl enable --now control-panel
```

Open `http://<server>:9090`. Every option is documented inline in
[config.example.yaml](config.example.yaml).

## Upgrading

```sh
make release
sudo systemctl stop control-panel        # gracefully stops managed MC servers
sudo install -m 0755 dist/control-panel-linux-amd64 /usr/local/bin/control-panel
sudo systemctl start control-panel       # resumes MC servers that were running
```

## Minecraft servers

Point `minecraft.root` at a directory laid out like:

```
/srv/minecraft/
├── survival/          # id: "survival"
│   ├── paper-1.21.4-115.jar
│   ├── server.properties
│   └── world/…
└── atm10/             # id: "atm10"
    ├── run.sh         # modded servers with launch scripts work too
    └── …
```

The panel launches servers itself (they are its child processes), which is
what enables the live console, stdin commands, and crash detection. Panel
restarts stop the servers gracefully and start them again afterwards
(`minecraft.resume`). Jar detection order: explicit `jar:` config →
`server.jar` → a single `*.jar` → `run.sh`.

Backups land in `<root>/.backups/<id>/` as `tar.gz`, excluding logs and
caches. Restores refuse to run while the server is up and keep the replaced
directory under `.backups/<id>/replaced-<timestamp>/`.

## Network exposure

The panel is designed for LAN use. For remote access prefer a VPN
(Tailscale/WireGuard) over port-forwarding. If you must expose it:

- set `tls.cert`/`tls.key` (or reverse-proxy with TLS),
- keep `auth.mode: password` (argon2id, rate-limited),
- consider `services.allow_actions: false` and `power.allow: false`.

## Where things live

| Path | Purpose |
|---|---|
| `/usr/local/bin/control-panel` | the single binary (UI embedded) |
| `/etc/control-panel/config.yaml` | configuration (0600, contains secrets) |
| `/var/lib/control-panel/` | sessions, audit log, UI prefs, MC overrides |
| `<minecraft.root>/.backups/` | Minecraft backups |
