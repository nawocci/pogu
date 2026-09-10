package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nawocci/pogu/internal/config"
	"github.com/nawocci/pogu/internal/control"
	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/httpapi"
	"github.com/nawocci/pogu/internal/provider"
	"github.com/nawocci/pogu/internal/service"
	"github.com/nawocci/pogu/internal/store"
	"github.com/nawocci/pogu/internal/telemetry"
	"github.com/nawocci/pogu/internal/web"
)

func initCommand(dataDir string, args []string) error {
	fs := newFlagSet("pogu init")
	password := os.Getenv("POGU_ADMIN_PASSWORD")
	fs.StringVar(&password, "password", password, "administrator password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if password == "" {
		return errors.New("--password or POGU_ADMIN_PASSWORD is required")
	}
	cfg := config.Default(dataDir)
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	key, err := crypto.LoadOrCreateKey(cfg.MasterKeyFile)
	if err != nil {
		return err
	}
	st, err := store.Open(cfg.DatabaseFile)
	if err != nil {
		return err
	}
	defer st.Close()
	svc := service.New(st, key)
	if err := svc.InitializeAdmin(context.Background(), password); err != nil {
		return err
	}
	if err := svc.EnsureBuiltin(context.Background()); err != nil {
		return err
	}
	fmt.Printf("initialized data directory %s\n", cfg.DataDir)
	return nil
}

func syncOpenCodeLoop(ctx context.Context, svc *service.Service, logger *slog.Logger) {
	ticker := time.NewTicker(7 * 24 * time.Hour)
	defer ticker.Stop()
	syncOnce := func() {
		p, err := svc.GetBuiltinProvider(ctx)
		if err != nil {
			logger.Warn("opencode catalog sync skipped", "error", err.Error())
			return
		}
		if !p.Enabled {
			return
		}
		syncCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		summary, err := svc.AutoSyncOpenCodeModels(syncCtx)
		if err != nil {
			logger.Warn("opencode catalog sync failed", "error", err.Error())
			return
		}
		logger.Info("opencode catalog sync", "added", summary.Added, "disabled", summary.Disabled, "free", summary.Free)
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(15 * time.Second):
	}
	syncOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncOnce()
		}
	}
}

func syncCavemanLoop(ctx context.Context, svc *service.Service, logger *slog.Logger) {
	ticker := time.NewTicker(7 * 24 * time.Hour)
	defer ticker.Stop()
	syncOnce := func() {
		syncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := svc.SyncCavemanSkill(syncCtx); err != nil {
			logger.Warn("caveman skill sync failed", "error", err.Error())
			return
		}
		logger.Info("caveman skill sync completed")
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}
	syncOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncOnce()
		}
	}
}
func openRuntime(dataDir string) (config.Config, *store.Store, *service.Service, bool, error) {
	cfg, err := config.Load(dataDir)
	if err != nil {
		return config.Config{}, nil, nil, false, err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return config.Config{}, nil, nil, false, fmt.Errorf("create data directory: %w", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "config.json")); err != nil {
		if !os.IsNotExist(err) {
			return config.Config{}, nil, nil, false, err
		}
		if err := cfg.Save(); err != nil {
			return config.Config{}, nil, nil, false, err
		}
	}
	key, err := crypto.LoadOrCreateKey(cfg.MasterKeyFile)
	if err != nil {
		return config.Config{}, nil, nil, false, err
	}
	st, err := store.Open(cfg.DatabaseFile)
	if err != nil {
		return config.Config{}, nil, nil, false, err
	}
	svc := service.New(st, key)
	if docsURL := os.Getenv("POGU_OPENCODE_DOCS_URL"); docsURL != "" {
		svc.OpenCodeDocsURL = docsURL
	}
	svc.OpenCodeDocsCacheFile = filepath.Join(cfg.DataDir, service.OpenCodeDocsCacheFile)
	if err := svc.EnsureBuiltin(context.Background()); err != nil {
		_ = st.Close()
		return config.Config{}, nil, nil, false, err
	}
	initialized, err := svc.AdminInitialized(context.Background())
	if err != nil {
		_ = st.Close()
		return config.Config{}, nil, nil, false, err
	}
	return cfg, st, svc, !initialized, nil
}

func serveCommand(dataDir string, args []string) error {
	fs := newFlagSet("pogu serve")
	listen := ""
	fs.StringVar(&listen, "listen", "", "HTTP listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, st, svc, setupMode, err := openRuntime(dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	if listen != "" {
		cfg.Listen = listen
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	upstream := provider.NewClient()
	controlServer := control.New(svc, upstream)
	controlServer.Telemetry = telemetry.NewRecorder(svc.Store.DB)
	if err := controlServer.Start(cfg.ControlSock); err != nil {
		return err
	}
	defer controlServer.Close()
	api := httpapi.New(svc, logger)
	if setupMode {
		token, err := httpapi.GenerateSetupToken()
		if err != nil {
			return fmt.Errorf("generate setup token: %w", err)
		}
		api.EnableSetup(token)
		logger.Warn("initial setup required",
			"hint", "open the admin UI in a browser and complete setup with the token below")
		logger.Warn("setup token", "token", token)
	}
	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.Handler(web.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	logger.Info("server starting", "addr", cfg.Listen, "control_socket", cfg.ControlSock, "data_dir", cfg.DataDir)
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go syncOpenCodeLoop(ctx, svc, logger)
	go syncCavemanLoop(ctx, svc, logger)
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	}
}
