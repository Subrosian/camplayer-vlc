package main

import (
	"bufio"
	"context"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const configPath = "/etc/camplayer-vlc.conf"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[camplayer-vlc] ")

	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if cfg.RTSP_URL == "" {
		log.Fatalf("RTSP_URL is required in %s", configPath)
	}

	log.Printf("Config loaded: RTSP_URL=%s VLC_PATH=%s EXTRA_ARGS=%v",
		cfg.RTSP_URL, cfg.VLC_PATH, cfg.VLC_EXTRA_ARGS)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := runLoop(ctx, cfg); err != nil {
		log.Fatalf("Supervisor exiting: %v", err)
	}
}

type Config struct {
	RTSP_URL       string
	VLC_PATH       string
	VLC_EXTRA_ARGS []string
}

func loadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := &Config{
		VLC_PATH: "vlc", // default
		VLC_EXTRA_ARGS: []string{
			"-I", "dummy",
			"--no-osd",
			"--no-video-title-show",
			"--rtsp-tcp",
			"--network-caching=300",
