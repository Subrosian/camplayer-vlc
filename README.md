## Overview
This is a simple "vibe-coded" app written in Go that boots up a Raspberry Pi and displays an RTSP stream on the HDMI port. It takes inspiration from the Camplayer project, but works on newer OS versions and with only the features that I personally needed (basically none).

## Installation
Set up a default environment of "Raspberry Pi OS Lite 64-bit (Trixie)"
Make sure you use "pi" as the username

Download the installer
```
wget https://github.com/Subrosian/camplayer-vlc/raw/refs/heads/main/install.sh
```

Run the installer
```
sudo bash ./install.sh
```

Edit the config file
```
sudo nano /etc/camplayer-vlc.conf
```

Restart camplayer-vlc service
```
sudo systemctl restart camplayer-vlc.service
```

## Updating

Run the installer again (locally)
```
sudo bash /opt/camplayer-vlc/install.sh
```