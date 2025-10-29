package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Jeomhps/projet-IAC/scheduler-go/internal/cleanup"
	"github.com/Jeomhps/projet-IAC/scheduler-go/internal/config"
	"github.com/Jeomhps/projet-IAC/scheduler-go/internal/db"
	"github.com/Jeomhps/projet-IAC/scheduler-go/internal/lock"
	"github.com/Jeomhps/projet-IAC/scheduler-go/internal/runner"
)

func main() {
	cfg := config.Load()

	once := flag.Bool("once", false, "Run cleanup once and exit")
	interval := flag.Int("interval", cfg.IntervalSec, "Loop interval in seconds")
	flag.Parse()
	if *once {
		cfg.Once = true
	}
	cfg.IntervalSec = *interval

	// Configure Ansible verbosity via env based on LOG_LEVEL
	level := logLevel()
	setAnsibleEnvForLevel(level)

	d, err := db.Open(cfg.DSN())
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer d.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	run := func() {
		sqlStd := d.DB.DB
		a, err := lock.Acquire(sqlStd, cfg.LockName, cfg.LockTimeout)
		if err != nil {
			log.Printf("skip run: %v", err)
			return
		}
		defer a.Release()

		cl := cleanup.Cleaner{
			DB:        d,
			Runner:    runner.PlaybookRunner{Playbook: cfg.PlaybookPath, Forks: cfg.Forks},
			TempDir:   cfg.TempDir,
			BatchSize: atoi(getenv("CLEANUP_BATCH_SIZE", "20"), 20),
		}
		n, err := cl.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("cleanup error: %v", err)
			return
		}
		if n > 0 {
			log.Printf("Cleaned up %d expired reservations.", n)
		} else {
			log.Printf("No expired reservations to clean up.")
		}
	}

	if cfg.Once {
		run()
		return
	}

	log.Printf("Starting maintenance loop (interval=%ds)", cfg.IntervalSec)
	t := time.NewTicker(time.Duration(cfg.IntervalSec) * time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Scheduler exiting gracefully")
			return
		case <-t.C:
			run()
		}
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func atoi(s string, def int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// ---- Logging helpers ----

func logLevel() string {
	v := os.Getenv("LOG_LEVEL")
	if v == "" {
		return "info"
	}
	return v
}

func setAnsibleEnvForLevel(level string) {
	// Always keep color if logs support it
	_ = os.Setenv("ANSIBLE_FORCE_COLOR", "1")

	switch level {
	case "trace3", "trace-3":
		_ = os.Setenv("ANSIBLE_VERBOSITY", "3")
		log.Printf("LOG_LEVEL=%s -> ANSIBLE_VERBOSITY=3", level)
	case "trace2", "trace-2":
		_ = os.Setenv("ANSIBLE_VERBOSITY", "2")
		log.Printf("LOG_LEVEL=%s -> ANSIBLE_VERBOSITY=2", level)
	case "trace", "trace1", "trace-1":
		_ = os.Setenv("ANSIBLE_VERBOSITY", "1")
		log.Printf("LOG_LEVEL=%s -> ANSIBLE_VERBOSITY=1", level)
	case "debug":
		// concise default output, but ensure colored human-friendly formatter
		// leave verbosity unset to avoid -v noise
		_ = os.Setenv("ANSIBLE_STDOUT_CALLBACK", "yaml")
		log.Printf("LOG_LEVEL=debug -> human-readable Ansible output (no extra verbosity)")
	default: // info
		// no extra env; Ansible defaults
	}
}
