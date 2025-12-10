package main

import (
	"bufio"
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const configPath = "/etc/camplayer-vlc.conf"

type Config struct {
	RTSP_URL string
	VLC_PATH string
}

var (
	cfgMu sync.Mutex
)

// ---- main ----

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[camplayer-vlc] ")

	// Initial config load (just to fail fast if config unreadable)
	if _, err := loadConfig(); err != nil {
		log.Printf("Warning: initial config load failed: %v (will retry in loop)", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start web server in background
	go func() {
		if err := startWebServer(ctx); err != nil {
			log.Printf("Web server error: %v", err)
		}
	}()

	// Run VLC supervisor loop (blocks until shutdown)
	if err := runLoop(ctx); err != nil {
		log.Fatalf("Supervisor exiting with error: %v", err)
	}
}

// ---- config helpers ----

func loadConfig() (*Config, error) {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	f, err := os.Open(configPath)
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

	// Defaults
	if cfg.VLC_PATH == "" {
		cfg.VLC_PATH = "cvlc"
	}

	return cfg, nil
}

func saveConfig(newCfg *Config) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	// Very simple writer: overwrite with only these keys
	f, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	if newCfg.RTSP_URL != "" {
		if _, err := writer.WriteString("RTSP_URL=" + newCfg.RTSP_URL + "\n"); err != nil {
			return err
		}
	}
	if newCfg.VLC_PATH != "" {
		if _, err := writer.WriteString("VLC_PATH=" + newCfg.VLC_PATH + "\n"); err != nil {
			return err
		}
	}

	return writer.Flush()
}

// ---- VLC supervisor loop ----

func runLoop(ctx context.Context) error {
	backoff := 2 * time.Second
	const maxBackoff = 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			log.Println("Shutdown requested, exiting supervisor loop")
			return nil
		default:
		}

		cfg, err := loadConfig()
		if err != nil {
			log.Printf("Failed to load config, retrying in %s: %v", backoff, err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			// increase backoff and continue
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			continue
		}

		if cfg.RTSP_URL == "" {
			log.Printf("RTSP_URL is empty in %s, retrying in %s", configPath, backoff)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			continue
		}

		// Reset backoff after a successful config read
		backoff = 2 * time.Second

		args := []string{cfg.RTSP_URL}
		cmd := exec.CommandContext(ctx, cfg.VLC_PATH, args...)
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

// ---- web UI ----

var pageTmpl = template.Must(template.New("page").Parse(`
<!doctype html>
<html>
<head>
	<meta charset="utf-8">
	<title>camplayer-vlc configuration</title>
	<style>
		body { font-family: sans-serif; max-width: 600px; margin: 2rem auto; }
		label { display: block; margin-bottom: 0.5rem; font-weight: bold; }
		input[type=text] { width: 100%; padding: 0.5rem; }
		button { margin-top: 1rem; padding: 0.5rem 1rem; }
		.msg { color: green; margin-bottom: 1rem; }
		.error { color: red; margin-bottom: 1rem; }
	</style>
</head>
<body>
	<h1>camplayer-vlc configuration</h1>

	{{if .Message}}
	<div class="msg">{{.Message}}</div>
	{{end}}
	{{if .Error}}
	<div class="error">{{.Error}}</div>
	{{end}}

	<form method="POST" action="/update">
		<label for="rtsp_url">RTSP URL</label>
		<input type="text" id="rtsp_url" name="rtsp_url" value="{{.RTSP_URL}}">
		<button type="submit">Save</button>
	</form>

	<p style="margin-top:2rem;font-size:0.9rem;color:#555;">
		Changes are saved to <code>/etc/camplayer-vlc.conf</code> and will be used
		the next time VLC is (re)started by the supervisor.
	</p>
</body>
</html>
`))

func startWebServer(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := loadConfig()
		data := struct {
			RTSP_URL string
			Message  string
			Error    string
		}{
			RTSP_URL: "",
		}
		if err != nil {
			data.Error = "Failed to load config: " + err.Error()
		} else {
			data.RTSP_URL = cfg.RTSP_URL
		}
		if err := pageTmpl.Execute(w, data); err != nil {
			log.Printf("Template execute error: %v", err)
		}
	})

	mux.HandleFunc("/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad form", http.StatusBadRequest)
			return
		}
		rtsp := strings.TrimSpace(r.Form.Get("rtsp_url"))
		data := struct {
			RTSP_URL string
			Message  string
			Error    string
		}{
			RTSP_URL: rtsp,
		}
		if rtsp == "" {
			data.Error = "RTSP URL cannot be empty"
			if err := pageTmpl.Execute(w, data); err != nil {
				log.Printf("Template execute error: %v", err)
			}
			return
		}

		// Preserve existing VLC_PATH if present
		cfg, err := loadConfig()
		if err != nil {
			// if we can't load, just use defaults
			cfg = &Config{}
		}
		cfg.RTSP_URL = rtsp
		if cfg.VLC_PATH == "" {
			cfg.VLC_PATH = "cvlc"
		}

		if err := saveConfig(cfg); err != nil {
			data.Error = "Failed to save config: " + err.Error()
		} else {
			data.Message = "Configuration saved."
		}

		if err := pageTmpl.Execute(w, data); err != nil {
			log.Printf("Template execute error: %v", err)
		}
	})

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Shutdown when context is cancelled
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Println("Web UI listening on http://0.0.0.0:8080")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
