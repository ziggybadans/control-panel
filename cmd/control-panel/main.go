// Command control-panel is a self-hosted server control panel: system
// metrics, NAS storage, systemd services, Plex, and Minecraft management
// behind a single embedded web UI.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/ziggybadans/control-panel/internal/apps"
	"github.com/ziggybadans/control-panel/internal/audit"
	"github.com/ziggybadans/control-panel/internal/auth"
	"github.com/ziggybadans/control-panel/internal/config"
	"github.com/ziggybadans/control-panel/internal/events"
	"github.com/ziggybadans/control-panel/internal/fans"
	"github.com/ziggybadans/control-panel/internal/files"
	"github.com/ziggybadans/control-panel/internal/httpd"
	"github.com/ziggybadans/control-panel/internal/jobs"
	"github.com/ziggybadans/control-panel/internal/mc"
	"github.com/ziggybadans/control-panel/internal/metrics"
	"github.com/ziggybadans/control-panel/internal/plex"
	"github.com/ziggybadans/control-panel/internal/prefs"
	"github.com/ziggybadans/control-panel/internal/qbit"
	"github.com/ziggybadans/control-panel/internal/sched"
	"github.com/ziggybadans/control-panel/internal/services"
	"github.com/ziggybadans/control-panel/internal/storage"
	cpterm "github.com/ziggybadans/control-panel/internal/term"
	"github.com/ziggybadans/control-panel/internal/update"
)

var version = "dev" // set via -ldflags at build time

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hash":
			hashCommand()
			return
		case "version":
			fmt.Println("control-panel", version)
			return
		}
	}

	var (
		configPath = flag.String("config", "", "path to config.yaml")
		mockFlag   = flag.Bool("mock", false, "serve synthetic data (UI development)")
		listenFlag = flag.String("listen", "", "override listen address")
	)
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(err)
	}
	if *listenFlag != "" {
		cfg.Listen = *listenFlag
	}

	mock := *mockFlag
	if !mock && runtime.GOOS != "linux" {
		slog.Warn("non-Linux host: forcing --mock (real providers need Linux)")
		mock = true
	}
	if !mock && cfg.Auth.Mode == "password" && cfg.Auth.PasswordHash == "" {
		fatal(fmt.Errorf("no auth.password_hash configured.\n" +
			"Generate one with:  control-panel hash\n" +
			"Or (NOT recommended outside trusted networks) set auth.mode: none"))
	}
	if !mock && cfg.TLS.Cert == "" && !cfg.ListenIsLoopback() {
		slog.Warn("no TLS on a non-loopback listen: the panel password crosses the " +
			"network in cleartext — configure tls.cert/key, or bind listen to a " +
			"loopback/VPN address (e.g. a tailscale IP)")
	}
	if !mock && cfg.Minecraft.RunAs != "" {
		u, err := user.Lookup(cfg.Minecraft.RunAs)
		if err != nil {
			fatal(fmt.Errorf("minecraft.run_as: unknown user %q: %w", cfg.Minecraft.RunAs, err))
		}
		uid, err1 := strconv.Atoi(u.Uid)
		gid, err2 := strconv.Atoi(u.Gid)
		if err1 != nil || err2 != nil {
			fatal(fmt.Errorf("minecraft.run_as: non-numeric uid/gid for %q", cfg.Minecraft.RunAs))
		}
		if uid == 0 {
			fatal(fmt.Errorf("minecraft.run_as must name an unprivileged user, not root"))
		}
		if os.Geteuid() != 0 {
			fatal(fmt.Errorf("minecraft.run_as requires the panel itself to run as root"))
		}
		cfg.Minecraft.RunAsUID, cfg.Minecraft.RunAsGID = uid, gid
		slog.Info("minecraft servers will run de-privileged", "user", cfg.Minecraft.RunAs, "uid", uid, "gid", gid)
	}

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		fatal(fmt.Errorf("create data dir %s: %w", cfg.DataDir, err))
	}

	bus := events.NewBus()
	runner := jobs.NewRunner()
	runner.Notify = func() { bus.Publish("jobs", runner.List()) }

	// Providers: real (Linux) or mock.
	var (
		collector   metrics.Collector
		storageProv storage.Provider
		serviceProv services.Provider
		plexProv    plex.Provider
		appsProv    apps.Provider
		qbitProv    qbit.Provider
		mcService   mc.Service
		mcManager   *mc.Manager
		updateProv  update.Provider
		fansProv    fans.Provider
		powerFn     func(string) error
	)
	if mock {
		collector = metrics.NewMockCollector(version)
		storageProv = storage.NewMockProvider()
		serviceProv = services.NewMockProvider()
		plexProv = plex.NewMockProvider()
		appsProv = apps.NewMockProvider()
		qbitProv = qbit.NewMockProvider()
		mcService = mc.NewMockService(bus, runner, cfg.DataDir)
		updateProv = update.NewMockProvider(version)
		fansProv = fans.NewMockProvider()
		powerFn = func(action string) error {
			slog.Info("mock power action (no-op)", "action", action)
			return nil
		}
	} else {
		collector = newLinuxCollector(version)
		storageProv = newLinuxStorage(cfg.Storage)
		serviceProv = newSystemdProvider(cfg.Services.Units)
		plexProv = plex.NewClient(cfg.Plex.URL, cfg.Plex.Token)
		appsProv = apps.NewClient(cfg.Apps)
		qbitProv = qbit.NewClient(cfg.QBittorrent.URL, cfg.QBittorrent.Username,
			cfg.QBittorrent.Password, cfg.QBitActions())
		mcManager = mc.NewManager(cfg.Minecraft, cfg.DataDir, bus, runner)
		mcService = mcManager
		updateProv = update.NewClient(cfg.Update.Repo, cfg.Update.Token, version)
		fansProv = newLinuxFans()
		powerFn = func(action string) error {
			verb := map[string]string{"reboot": "reboot", "shutdown": "poweroff"}[action]
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, "systemctl", "--no-ask-password", verb).CombinedOutput()
			if err != nil {
				return fmt.Errorf("systemctl %s: %s", verb, strings.TrimSpace(string(out)))
			}
			return nil
		}
	}

	sysInfo, err := collector.Info(context.Background())
	if err != nil {
		slog.Warn("system info collection failed", "err", err)
	}
	sysInfo.Version = version
	sysInfo.Mock = mock

	sampler := metrics.NewSampler(collector, bus,
		time.Duration(cfg.Metrics.IntervalMS)*time.Millisecond, cfg.Metrics.History)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go sampler.Run(ctx)

	fansCtl := fans.NewController(fansProv, bus, cfg.DataDir,
		time.Duration(cfg.Fans.IntervalMS)*time.Millisecond, cfg.FanControl())
	go fansCtl.Run(ctx)

	// File manager roots: in mock mode with no explicit config, browse a
	// seeded sandbox tree instead of the real filesystem.
	filesRoots := cfg.Files.Roots
	if mock && len(filesRoots) == 0 {
		if seeded, err := files.SeedMock(cfg.DataDir); err == nil {
			filesRoots = seeded
		} else {
			slog.Warn("mock file tree seeding failed", "err", err)
		}
	}
	filesSvc := files.New(filesRoots)

	// Terminal: mock mode gets a simulated shell; real mode builds a PTY
	// launcher (with run_as safety checks) only when explicitly enabled.
	var termLauncher cpterm.Launcher
	if cfg.Terminal.Enabled {
		if mock {
			termLauncher = cpterm.NewMockLauncher()
		} else {
			l, err := newTermLauncher(cfg)
			if err != nil {
				fatal(err)
			}
			termLauncher = l
		}
	}
	termMgr := cpterm.NewManager(termLauncher, cfg.Terminal.MaxSessions,
		time.Duration(cfg.Terminal.IdleTimeoutMin)*time.Minute)

	// Scheduled tasks: allowlisted panel actions on a recurrence. Every
	// execution is audited with "scheduler" as the source.
	auditLog := audit.New(cfg.DataDir)
	svcUnits := cfg.Services.Units
	if len(svcUnits) == 0 {
		svcUnits = services.DefaultUnits
	}
	schedEngine := sched.NewEngine(cfg.DataDir, &schedExecutor{
		mc:       mcService,
		services: serviceProv,
		storage:  storageProv,
		runner:   runner,
		units:    svcUnits,
	})
	schedEngine.OnRun = func(s sched.Schedule, detail string, err error) {
		e := audit.Entry{
			IP: "scheduler", Action: "sched.run", Target: s.Name,
			Detail: s.Action + ": " + detail, OK: err == nil,
		}
		if err != nil {
			e.Err = err.Error()
			e.Detail = s.Action
		}
		auditLog.Record(e)
	}
	go schedEngine.Run(ctx)

	// Service state watcher: cheap poll, published to all SSE clients.
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if bus.Subscribers() == 0 {
					continue
				}
				if list, err := serviceProv.List(ctx); err == nil {
					bus.Publish("services", list)
				}
			}
		}
	}()

	server := httpd.New(httpd.Deps{
		Cfg:      cfg,
		Version:  version,
		Bus:      bus,
		Sessions: auth.NewSessions(cfg.SessionTTL(), cfg.DataDir),
		Limiter:  auth.NewLoginLimiter(5, 15*time.Minute),
		Audit:    auditLog,
		Prefs:    prefs.New(cfg.DataDir),
		Jobs:     runner,
		Sampler:  sampler,
		SysInfo:  sysInfo,
		Storage:  storageProv,
		Services: serviceProv,
		Plex:     plexProv,
		Apps:     appsProv,
		Qbit:     qbitProv,
		MC:       mcService,
		Update:   updateProv,
		Fans:     fansCtl,
		Files:    filesSvc,
		Term:     termMgr,
		Sched:    schedEngine,
		Power:    powerFn,
	})

	if mcManager != nil {
		go mcManager.StartupResume(cfg.ResumeMC())
	}

	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()

	select {
	case err := <-errCh:
		fatal(err)
	case <-ctx.Done():
		slog.Info("shutting down")
		termMgr.CloseAll()
		// Hand fans back to firmware before exiting: a dead panel must
		// never leave a fan pinned at a low manual duty.
		fansCtl.ReleaseAll()
		if mcManager != nil {
			// Stop game servers cleanly before the panel exits (children
			// die with the panel process otherwise).
			mcManager.GracefulShutdown(2 * time.Minute)
		}
	}
}

func hashCommand() {
	var pw string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Password: ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			fatal(err)
		}
		fmt.Fprint(os.Stderr, "Confirm:  ")
		b2, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			fatal(err)
		}
		if string(b) != string(b2) {
			fatal(fmt.Errorf("passwords do not match"))
		}
		pw = string(b)
	} else {
		sc := bufio.NewScanner(os.Stdin)
		if sc.Scan() {
			pw = strings.TrimSpace(sc.Text())
		}
	}
	if len(pw) < 8 {
		fatal(fmt.Errorf("password must be at least 8 characters"))
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		fatal(err)
	}
	fmt.Print("\nAdd to your config.yaml:\n\n")
	fmt.Printf("auth:\n  password_hash: \"%s\"\n", hash)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "control-panel:", err)
	os.Exit(1)
}
