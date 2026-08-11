#!/bin/sh
# Installs control-panel onto a Debian server.
# Run from the repo root (or a release directory) as root:
#   sudo ./deploy/install.sh [path-to-binary]
set -eu

BIN="${1:-dist/control-panel-linux-amd64}"
if [ ! -f "$BIN" ]; then
    echo "binary not found: $BIN" >&2
    echo "build it first:  make release" >&2
    exit 1
fi
if [ "$(id -u)" != "0" ]; then
    echo "run as root (sudo)" >&2
    exit 1
fi

echo "-> installing binary to /usr/local/bin/control-panel"
install -m 0755 "$BIN" /usr/local/bin/control-panel

echo "-> creating /etc/control-panel and /var/lib/control-panel"
install -d -m 0755 /etc/control-panel
install -d -m 0700 /var/lib/control-panel
if [ ! -f /etc/control-panel/config.yaml ]; then
    install -m 0600 deploy/config.example.yaml /etc/control-panel/config.yaml
    echo "   wrote /etc/control-panel/config.yaml (from example)"
else
    echo "   keeping existing /etc/control-panel/config.yaml"
fi

echo "-> installing systemd unit"
install -m 0644 deploy/control-panel.service /etc/systemd/system/control-panel.service
systemctl daemon-reload

cat <<'EOF'

Installed. Next steps:

  1. Set a panel password:
         control-panel hash
     then paste the printed hash into /etc/control-panel/config.yaml
     (auth.password_hash).

  2. Review the rest of the config (minecraft.root, services.units,
     plex.token, storage.snapraid.config).

  3. Start it:
         systemctl enable --now control-panel

  4. Open http://<server>:9090 from your Mac/PC.
EOF
