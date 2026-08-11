# Panel Bar — macOS menu bar companion

A native menu bar app showing your server at a glance: CPU, memory, pool
capacity, Minecraft servers with player counts, Plex streams, and health
warnings (SMART, failed services, crashes). Click through to the full panel.

The menu bar item shows the CPU percentage (optional) and swaps to a warning
glyph when something needs attention or the panel is unreachable.

## Build & install

```sh
make mac-bar                       # from the repo root
cp -r "dist/Panel Bar.app" /Applications/
open "/Applications/Panel Bar.app"
```

Requires macOS 13+ and the Xcode Command Line Tools to build.

## Setup

Click the menu bar item → gear icon:

- **Panel URL** — e.g. `http://bastion:9090` (or a Tailscale address)
- **Password** — the panel password; stored in the macOS Keychain, exchanged
  for a session cookie (never sent anywhere but your panel)
- **Refresh** — poll cadence (the endpoint is one cheap composed payload)
- **Launch at login** — registers with the system via `SMAppService`

The app is ad-hoc signed by the build. First launch may need
right-click → Open (unsigned-developer warning), one time only.
