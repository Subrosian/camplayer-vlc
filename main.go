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

type Config struct {
	RTSP_URL       string
	VLC_PATH       string
	VLC_EXTRA_ARGS []string
}

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

	log.Printf("Config: RTSP_URL=%s VLC_PATH=%s EXTRA_ARGS=%v",
		cfg.RTSP_URL, cfg.VLC_PATH, cfg.VLC_EXTRA_ARGS)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := runLoop(ctx, cfg); err != nil {
		log.Fatalf("Supervisor exiting: %v", err)
	}
}

func loadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := &Config{
		VLC_PATH: "vlc",
		VLC_EXTRA_ARGS: []string{
			"-I", "dummy",
			"--no-osd",
			"--no-video-title-show",
			"--rtsp-tcp",
			"--network-caching=300",
			"--sout-keep",
		},
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			log.Printf("Ignoring malformed config line: %s", line)
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "RTSP_URL":
			cfg.RTSP_URL = val

		case "VLC_PATH":
			if val != "" {
				cfg.VLC_PATH = val
			}

		case "VLC_EXTRA_ARGS":
			if val != "" {
				cfg.VLC_EXTRA_ARGS = strings.Fields(val)
			}
		}
	}

	return cfg, scanner.Err()
}

func runLoop(ctx context.Context, cfg *Config) error {
	backoff := 2 * time.Second
	const maxBackoff = 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			log.Println("Shutdown requested")
			return nil
		default:
		}

		args := append(append([]string{}, cfg.VLC_EXTRA_ARGS...), cfg.RTSP_URL)
		cmd := exec.CommandContext(ctx, cfg.VLC_PATH, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		log.Printf("Launching VLC: %s %s", cfg.VLC_PATH, strings.Join(args, " "))

		if err := cmd.Start(); err != nil {
			log.Printf("Failed to start VLC: %v", err)
		} else {
			err := cmd.Wait()
			if ctx.Err() != nil {
				log.Println("Context canceled; stopping supervisor.")
				return nil
			}
			log.Printf("VLC exited: %v", err)
		}

		log.Printf("Restarting VLC in %s...", backoff)

		select {
		case <-ctx.Done():
			log.Println("Shutdown requested during backoff")
			return nil
		case <-time.After(backoff):
		}

		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}
