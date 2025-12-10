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
	RTSP_URL string
	VLC_PATH string
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[camplayer-vlc] ")

	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config %s: %v", configPath, err)
	}

	if cfg.RTSP_URL == "" {
		log.Fatalf("RTSP_URL is required in %s", configPath)
	}
	if cfg.VLC_PATH == "" {
		cfg.VLC_PATH = "vlc"
	}

	log.Printf("Config loaded: VLC_PATH=%s RTSP_URL=%s", cfg.VLC_PATH, cfg.RTSP_URL)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := runLoop(ctx, cfg); err != nil {
		log.Fatalf("Supervisor exiting with error: %v", err)
	}
}

func loadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := &Config{}
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and blank lines
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
			cfg.VLC_PATH = val
		default:
			log.Printf("Ignoring unknown config key: %s", key)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func runLoop(ctx context.Context, cfg *Config) error {
	backoff := 2 * time.Second
	const maxBackoff = 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			log.Println("Shutdown requested, exiting supervisor loop")
			return nil
		default:
		}

		// Command: vlc <rtsp-url>
		args := []string{cfg.RTSP_URL}
		cmd := exec.CommandContext(ctx, cfg.VLC_PATH, args...)

		// 🔑 IMPORTANT: give VLC the same stdin/stdout/stderr as your shell
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		log.Printf("Launching VLC: %s %s", cfg.VLC_PATH, cfg.RTSP_URL)

		if err := cmd.Start(); err != nil {
			log.Printf("Failed to start VLC: %v", err)
		} else {
			err := cmd.Wait()
			if ctx.Err() != nil {
				log.Println("Context cancelled while waiting for VLC; exiting.")
				return nil
			}
			log.Printf("VLC exited: %v", err)
		}

		log.Printf("Restarting VLC in %s...", backoff)
		select {
		case <-ctx.Done():
			log.Println("Shutdown requested during backoff; exiting.")
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
