#!/usr/bin/env bash
set -euo pipefail

if [[ "$EUID" -ne 0 ]]; then
	echo "Please run this script as root, e.g.: sudo $0" >&2
	exit 1
fi

echo "Updating package lists..."
apt update

echo "Installing dependencies (git, golang, vlc)..."
apt install -y git golang vlc

# Clone repo (into /opt by default)
if [[ ! -d /opt ]]; then
	mkdir -p /opt
fi

if [[ ! -d /opt/camplayer-vlc ]]; then
	echo "Cloning camplayer-vlc repository..."
	git clone https://github.com/Subrosian/camplayer-vlc /opt/camplayer-vlc
else
	echo "camplayer-vlc directory already exists, pulling latest changes..."
	cd /opt/camplayer-vlc
	git pull
fi

cd /opt/camplayer-vlc

echo "Building camplayer-vlc..."
go build -o camplayer-vlc .

if systemctl cat camplayer-vlc.service >/dev/null 2>&1; then
	echo "Stopping previously installed service to modify files..."
	systemctl stop camplayer-vlc.service
fi

echo "Installing camplayer-vlc binary to /usr/local/bin/..."
cp camplayer-vlc /usr/local/bin/
chown root:root /usr/local/bin/camplayer-vlc
chmod 755 /usr/local/bin/camplayer-vlc

echo "Creating systemd service file..."
cat >/etc/systemd/system/camplayer-vlc.service <<'EOF'
[Unit]
Description=camplayer-vlc RTSP player
After=network-online.target
Wants=network-online.target

[Service]
Type=simple

User=pi
Group=pi
WorkingDirectory=/home/pi
Environment=HOME=/home/pi

ExecStart=/usr/local/bin/camplayer-vlc

Restart=always
RestartSec=5

StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

echo "Creating default config file at /etc/camplayer-vlc.conf..."
if [[ ! -f /etc/camplayer-vlc.conf ]]; then
	cat >/etc/camplayer-vlc.conf <<'EOF'
RTSP_URL=rtsp://user:pass@camera-ip:554/stream
EOF
else
	echo "/etc/camplayer-vlc.conf already exists; leaving it unchanged."
fi
sudo chown pi:pi /etc/camplayer-vlc.conf
sudo chmod 664 /etc/camplayer-vlc.conf

echo "Reloading systemd daemon and enabling service..."
systemctl daemon-reload
systemctl enable camplayer-vlc.service

echo "Starting camplayer-vlc service..."
systemctl start camplayer-vlc.service

echo "Done. You can check status with: systemctl status camplayer-vlc.service"
