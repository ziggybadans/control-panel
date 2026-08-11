# Control Panel

A self-hosted control panel for a Debian home server running NAS storage
(mergerfs + snapraid), Plex, and multiple Minecraft servers.

Single static Go binary with an embedded web UI. Runs on the server as a
systemd service; accessed from any browser on the LAN.

```
┌─────────────┐   HTTPS / LAN    ┌──────────────────────────────────────┐
│ Mac / PC    │ ───────────────► │ control-panel (Go, single binary)    │
│ browser     │   REST + SSE     │  ├─ metrics    /proc, hwmon          │
└─────────────┘                  │  ├─ storage    mergerfs, SMART,      │
                                 │  │             snapraid              │
                                 │  ├─ services   systemd (allowlist)   │
                                 │  ├─ plex       Plex HTTP API         │
                                 │  └─ minecraft  process supervision,  │
                                 │                console, RCON,        │
                                 │                backups               │
                                 └──────────────────────────────────────┘
```

## Features

- **System** — live CPU (total + per-core), memory, load, network and disk
  throughput, temperatures, uptime. 1-second resolution over SSE.
- **Storage** — mergerfs pool with per-branch usage, all physical disks with
  SMART health / temperature / power-on hours, snapraid status with
  sync/scrub jobs (streamed output).
- **Services** — curated systemd units: state, uptime, memory, start / stop /
  restart, journal tail.
- **Plex** — active sessions (with transcode indicator), library statistics,
  service control.
- **Minecraft** — multiple servers, full lifecycle (start / stop / restart /
  kill), live console with command input, player management (kick / ban / op,
  whitelist), `server.properties` editor that preserves comments, JVM / memory
  configuration, tar.gz backups with restore, EULA handling, crash detection
  with optional auto-restart.
- **Customizable dashboard** — reorder / resize / hide widgets, accent color,
  density, dark & light themes. Preferences stored server-side, so they follow
  you across machines.
- **Audit log** — every action recorded with timestamp, IP, and outcome.
- **macOS menu bar app** — a native companion (`macbar/`) showing CPU,
  memory, pool usage, Minecraft players, Plex streams, and health warnings
  at a glance; password in the Keychain, one click to the full panel.

## Safety model

The panel is designed so it cannot damage the server it manages:

- **No arbitrary commands.** The agent executes a fixed allowlist of
  operations. Every external command is an argv array built from validated
  enums and IDs — user input is never interpolated into a shell.
- **Server-side confirmation.** Dangerous endpoints require an
  `X-Confirm: <target>` header echoing the exact target name; the UI enforces
  the same with typed-confirmation dialogs. Two tiers:
  - *confirm* — service stop/restart, Minecraft stop/restart
  - *typed confirm* — force-kill, backup restore/delete, snapraid sync/scrub,
    reboot/shutdown
- **Safe defaults.** Auth is mandatory unless explicitly disabled. Power
  actions are disabled until enabled in config. Backup restore refuses to run
  while the server is running and moves the replaced data aside instead of
  deleting it.
- **Read-only failure.** Metric collection failures degrade gracefully;
  they never block or affect the monitored workloads. The sampler idles when
  no client is connected.
- **Audit trail** for every mutating action.

## Quick start

```sh
# on your workstation (or the server)
make release            # builds ./dist/control-panel-linux-amd64

# on the server
sudo ./deploy/install.sh        # installs binary, config, systemd unit
control-panel hash              # generate a password hash, paste into config
sudo systemctl enable --now control-panel
```

See [deploy/README.md](deploy/README.md) for full installation and
configuration reference, and `deploy/config.example.yaml` for every option.

## Development

```sh
make dev        # backend in mock mode on :9090 + Vite dev server on :5173
make test       # Go unit tests + frontend type-check
make build      # production build for the host platform
make mac-bar    # macOS menu bar companion -> dist/Panel Bar.app
```

Mock mode (`--mock`) serves realistic synthetic data for every subsystem, so
the entire UI can be developed and demonstrated on macOS/Windows without a
Debian host.

## Stack

- **Backend** — Go, standard library only (plus `yaml` and `x/crypto` for
  argon2id). No cgo; static binary. SSE instead of WebSockets so the whole
  panel works with plain HTTP/1.1 and auto-reconnects.
- **Frontend** — React + TypeScript + Vite, hand-written CSS design system
  (no component framework), custom SVG charts. React Query for data,
  `useSyncExternalStore` for live streams.
