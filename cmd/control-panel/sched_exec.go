package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ziggybadans/control-panel/internal/jobs"
	"github.com/ziggybadans/control-panel/internal/mc"
	"github.com/ziggybadans/control-panel/internal/sched"
	"github.com/ziggybadans/control-panel/internal/services"
	"github.com/ziggybadans/control-panel/internal/storage"
)

// schedExecutor performs scheduled actions through the same providers the
// HTTP handlers use — a schedule can only ever do what the panel's
// allowlists already permit.
type schedExecutor struct {
	mc       mc.Service
	services services.Provider
	storage  storage.Provider
	runner   *jobs.Runner
	units    []string // services allowlist (config or defaults)
}

func (e *schedExecutor) ValidateTarget(s sched.Schedule) error {
	switch s.Action {
	case "mc.backup", "mc.restart", "mc.command":
		if _, ok := e.mc.Get(s.Server); !ok {
			return fmt.Errorf("unknown server %q", s.Server)
		}
	case "service.restart":
		return services.CheckUnit(e.units, s.Unit)
	case "snapraid.sync", "snapraid.scrub":
		if _, err := e.storage.SnapraidCmd(strings.TrimPrefix(s.Action, "snapraid.")); err != nil {
			return err
		}
	}
	return nil
}

func (e *schedExecutor) running(id string) bool {
	info, ok := e.mc.Get(id)
	return ok && (info.State == mc.StateRunning || info.State == mc.StateStarting)
}

func (e *schedExecutor) Run(ctx context.Context, s sched.Schedule) (string, error) {
	switch s.Action {
	case "mc.backup":
		if s.OnlyIfRunning && !e.running(s.Server) {
			return "skipped — server not running", nil
		}
		job, err := e.mc.CreateBackup(s.Server)
		if err != nil {
			return "", err
		}
		if s.Keep > 0 {
			go e.pruneWhenDone(job.ID, s.Server, s.Keep)
		}
		return "backup started", nil

	case "mc.restart":
		// A schedule must never surprise-start a stopped server.
		if !e.running(s.Server) {
			return "skipped — server not running", nil
		}
		if err := e.mc.Restart(s.Server); err != nil {
			return "", err
		}
		return "restarted", nil

	case "mc.command":
		if !e.running(s.Server) {
			return "skipped — server not running", nil
		}
		if err := e.mc.Command(s.Server, s.Command); err != nil {
			return "", err
		}
		return "ran: " + s.Command, nil

	case "service.restart":
		if err := services.CheckUnit(e.units, s.Unit); err != nil {
			return "", err
		}
		if err := e.services.Action(ctx, s.Unit, "restart"); err != nil {
			return "", err
		}
		return "restarted " + s.Unit, nil

	case "snapraid.sync", "snapraid.scrub":
		op := strings.TrimPrefix(s.Action, "snapraid.")
		argv, err := e.storage.SnapraidCmd(op)
		if err != nil {
			return "", err
		}
		_, err = e.runner.Start("snapraid."+op, "snapraid", func(jctx context.Context, out func(string)) error {
			return jobs.RunStreaming(jctx, argv, out)
		})
		if err != nil {
			return "", err
		}
		return "snapraid " + op + " started", nil
	}
	return "", fmt.Errorf("unknown action %q", s.Action)
}

// pruneWhenDone waits for the backup job to finish, then applies retention.
func (e *schedExecutor) pruneWhenDone(jobID, server string, keep int) {
	deadline := time.Now().Add(time.Hour)
	for time.Now().Before(deadline) {
		v, ok := e.runner.Get(jobID)
		if !ok {
			return // evicted from history; don't guess
		}
		if v.State != jobs.StateRunning {
			if v.State == jobs.StateDone {
				removed, err := e.mc.PruneBackups(server, keep)
				if err != nil {
					slog.Warn("backup retention failed", "server", server, "err", err)
				} else if len(removed) > 0 {
					slog.Info("pruned old backups", "server", server, "removed", len(removed))
				}
			}
			return
		}
		time.Sleep(5 * time.Second)
	}
}
