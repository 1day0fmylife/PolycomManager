package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"polycom-manager/internal/api"
	"polycom-manager/internal/config"
	cryptostore "polycom-manager/internal/crypto"
	"polycom-manager/internal/db"
	"polycom-manager/internal/device"
	"polycom-manager/internal/events"
	"polycom-manager/internal/sshclient"
	"polycom-manager/internal/webui"
)

type App struct {
	cfg     config.Config
	db      *sql.DB
	server  *http.Server
	manager *device.Manager
	log     *slog.Logger
}

func New(ctx context.Context, cfg config.Config, log *slog.Logger) (*App, error) {
	database, err := db.Open(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	crypto, err := cryptostore.NewStore(cfg.DataDir)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	repo := db.NewRepository(database, crypto)
	hostKeys, err := sshclient.NewHostKeyStore(cfg.DataDir)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	hub := events.New()
	manager := device.NewManager(repo, hostKeys, hub, log)
	if err := manager.Start(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	apiServer := api.New(ctx, repo, manager, hub, webui.Handler(), log)
	httpServer := &http.Server{Addr: cfg.Listen, Handler: apiServer.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	return &App{cfg: cfg, db: database, server: httpServer, manager: manager, log: log}, nil
}

func (a *App) Run() error {
	a.log.Info("Polycom Manager started", "listen", a.cfg.Listen, "data_dir", a.cfg.DataDir)
	if a.cfg.OpenUI && (len(a.cfg.Listen) >= 9 && (a.cfg.Listen[:9] == "127.0.0.1" || a.cfg.Listen[:9] == "localhost")) {
		go func() { time.Sleep(350 * time.Millisecond); _ = openBrowser("http://" + a.cfg.Listen) }()
	}
	err := a.server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (a *App) Shutdown(ctx context.Context) error {
	err := a.server.Shutdown(ctx)
	if a.db != nil {
		_ = a.db.Close()
	}
	return err
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
