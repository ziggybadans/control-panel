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
                                 │  ├─ fans       hwmon PWM curves      │
                                 │  ├─ files      confined file roots   │
                                 │  ├─ services   systemd (allowlist)   │
                                 │  ├─ plex       Plex HTTP API         │
                                 │  ├─ terminal   opt-in PTY (run_as)   │
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
- **Fans** — hwmon fan monitoring plus optional control: per-fan manual duty
  or an interactive temperature→duty curve editor (drag points, live
  operating marker). Fans stay under firmware control until explicitly taken
  over; unreadable sensors fail safe to 100%; the panel restores firmware
  control on shutdown.
- **Files** — a general file manager over config-defined roots (each
  optionally read-only): browse, drag-drop upload, rename/move, unzip,
  delete, raw file downloads, and folder downloads streamed as zip. All
  paths are confined to their root, symlinks included.
- **Terminal** — an opt-in web terminal (xterm.js over a server PTY) for the
  cases a panel button can't cover. Off by default, runs as an unprivileged
  `run_as` user, typed confirmation to open, session cap + idle timeout,
  open/close audited.
- **Services** — curated systemd units: state, uptime, memory, start / stop /
  restart, journal tail.
- **Plex** — active sessions (with transcode indicator), library statistics,
  service control.
- **Media apps** — Radarr / Sonarr / Lidarr / Readarr / Prowlarr and
  Overseerr / Jellyseerr: download queues with live progress, health
  warnings, missing and upcoming counts, pending requests. Read-only —
  API keys never leave the server; one click opens each app's own UI.
- **Minecraft** — multiple servers, full lifecycle (start / stop / restart /
  kill), live console with command input, player management (kick / ban / op,
  whitelist), `server.properties` editor that preserves comments, JVM / memory
  configuration, tar.gz backups with restore, EULA handling, crash detection
  with optional auto-restart. Plus: one-click **server setup** (Paper / Purpur /
  Fabric / Vanilla, official jars downloaded with streamed progress), a
  **file manager** confined to each server's directory (browse, rename/move,
  upload with drag-drop, download, zip/unzip, delete), a **plugins/mods
  manager** with metadata read from the jars and enable/disable toggles,
  **server jar updates** with rollback, and an embedded **map viewer** when
  Dynmap / BlueMap / squaremap / Pl3xMap is installed.
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
  enums and IDs — user input is never interpolated into a shell. The one
  deliberate exception is the opt-in Terminal page (a real shell): disabled
  by default, de-privileged via `terminal.run_as`, typed-confirmed, capped,
  idle-closed, and session-audited.
- **Server-side confirmation.** Dangerous endpoints require an
  `X-Confirm: <target>` header echoing the exact target name; the UI enforces
  the same with typed-confirmation dialogs. Two tiers:
  - *confirm* — service stop/restart, Minecraft stop/restart, fan settings
  - *typed confirm* — force-kill, backup restore/delete, snapraid sync/scrub,
    folder delete, terminal session, reboot/shutdown
- **Fail-safe fan control.** Fans are only driven when explicitly switched to
  manual/curve; a curve whose sensor disappears drives its fan to 100%, and
  release (or panel shutdown) restores the exact firmware state.
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
