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
	"github.com/Jeomhps/projet-IAC/scheduler-go/internal/enroll"
	"github.com/Jeomhps/projet-IAC/scheduler-go/internal/lock"
	"github.com/Jeomhps/projet-IAC/scheduler-go/internal/runner"
)

func main() {
	cfg := config.Load()

	once := flag.Bool("once", false, "Run maintenance once and exit")
	interval := flag.Int("interval", cfg.IntervalSec, "Loop interval in seconds")
	flag.Parse()
	if *once {
		cfg.Once = true
	}
	cfg.IntervalSec = *interval

	d, err := db.Open(cfg.DSN())
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer d.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	doEnroll := func(ctx context.Context) {
		s := enroll.Service{
			DB:                d.DB.DB,
			PlaybookPath:      cfg.EnrollPlaybookPath,
			ControllerKeyPath: cfg.EnrollKeyPrivatePath,
			TargetUser:        cfg.EnrollTargetUser,
			Forks:             cfg.Forks,
			TempDir:           cfg.TempDir,
		}
		n, err := s.Run(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("enroll error: %v", err)
			return
		}
		if n > 0 {
			log.Printf("Enrolled %d machine(s).", n)
		}
	}

	doCleanup := func(ctx context.Context) {
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

	run := func() {
		sqlStd := d.DB.DB
		a, err := lock.Acquire(sqlStd, cfg.LockName, cfg.LockTimeout)
		if err != nil {
			log.Printf("skip run: %v", err)
			return
		}
		defer a.Release()

		// Enrollment first, then cleanup
		doEnroll(ctx)
		doCleanup(ctx)
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
