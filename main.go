package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[camplayer-vlc] ")

	rtspURL := os.Getenv("RTSP_URL")
	if rtspURL == "" {
		log.Fatal("RTSP_URL is not set")
	}

	vlcPath := os.Getenv("VLC_PATH")
	if vlcPath == "" {
		vlcPath = "vlc"
	}

	extraArgsEnv := os.Getenv("VLC_EXTRA_ARGS")
	var extraArgs []string
	if extraArgsEnv != "" {
		extraArgs = splitArgs(extraArgsEnv)
	} else {
		extraArgs = []string{
			"-I", "dummy",
			"--no-osd",
			"--no-video-title-show",
			"--rtsp-tcp",
			"--network-caching=300",
			"--sout-keep",
		}
	}

	log.Printf("Starting supervisor. RTSP_URL=%s VLC_PATH=%s EXTRA_ARGS=%v", rtspURL, vlcPath, extraArgs)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := runLoop(ctx, vlcPath, rtspURL, extraArgs); err != nil {
		log.Fatalf("Supervisor exiting with error: %v", err)
	}
}

func runLoop(ctx context.Context, vlcPath, rtspURL string, baseArgs []string) error {
	backoff := 2 * time.Second
	const maxBackoff = 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			log.Println("Exiting supervisor loop")
			return nil
		default:
		}

		args := append(append([]string{}, baseArgs...), rtspURL)
		cmd := exec.CommandContext(ctx, vlcPath, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		log.Printf("Launching VLC: %s %s", vlcPath, strings.Join(args, " "))

		if err := cmd.Start(); err != nil {
			log.Printf("Failed to start VLC: %v", err)
		} else {
			err := cmd.Wait()
			if ctx.Err() != nil {
				log.Println("Context cancelled; exiting.")
				return nil
			}
			log.Printf("VLC exited: %v", err)
		}

		log.Printf("Restarting VLC in %s...", backoff)
		select {
		case <-ctx.Done():
			log.Println("Cancelled during backoff; exiting.")
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

func splitArgs(s string) []string {
	return strings.Fields(s)
}
