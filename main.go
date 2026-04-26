package main

import (
	"context"
	"crypto/rand"
	"embed"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"isley/app"
	"isley/config"
	"isley/handlers"
	"isley/logger"
	"isley/model"
	"isley/utils"
	"isley/watcher"
)

//go:embed model/migrations/sqlite/*.sql model/migrations/postgres/*.sql web/templates/**/*.html web/static/**/* utils/fonts/* VERSION
var embeddedFiles embed.FS

// trustedProxies trusts loopback and RFC-1918 private ranges so that
// c.ClientIP() returns the real client IP behind a reverse proxy. Users
// running without a proxy are unaffected.
var trustedProxies = []string{
	"127.0.0.1",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"::1",
}

func main() {
	logger.InitLogger()

	version := fmt.Sprintf("Isley %s", getVersion())
	logger.Log.Info("Starting application version:", version)

	port := os.Getenv("ISLEY_PORT")
	if port == "" {
		port = "8080"
	}

	model.MigrateDB()
	model.InitDB()
	model.RunStartupMaintenance()
	dbDriver := model.GetDriver()
	version = fmt.Sprintf("%s-%s", version, dbDriver)

	utils.Init("en")

	db, err := model.GetDB()
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to open database for credential init")
	}

	// Seed default admin credentials if absent.
	present, err := handlers.ExistsSetting(db, "auth_username")
	if err != nil {
		logger.Log.WithError(err).Error("Error checking if default admin credentials are present")
	} else if !present {
		handlers.UpdateSetting(db, "auth_username", "admin")
		hashedPassword, _ := utils.HashPassword("isley")
		handlers.UpdateSetting(db, "auth_password", hashedPassword)
		handlers.UpdateSetting(db, "force_password_change", "true")
	}

	// Load settings before pruning so config.SensorRetention is populated.
	handlers.LoadSettings(db)

	if config.SensorRetention <= 0 {
		logger.Log.Warn("Sensor data retention is disabled (sensor_retention_days = 0). " +
			"Sensor data will grow indefinitely. Consider setting a retention period " +
			"(e.g. 90 days) in Settings to prevent unbounded database growth.")
	}

	// Cancellable context for graceful shutdown of background goroutines.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Background services run inside this WaitGroup so shutdown can wait
	// for in-flight iterations to complete before exiting.
	var bgWG sync.WaitGroup

	w := watcher.New(db)

	// Prune old sensor data once before the watcher loop kicks in.
	if err := w.PruneSensorData(); err != nil {
		logger.Log.WithError(err).Error("Initial sensor data prune failed")
	} else {
		logger.Log.Info("Initial sensor data prune completed")
	}

	bgWG.Add(1)
	go func() {
		defer bgWG.Done()
		w.Run(ctx)
	}()

	// Resolve session secret. If unset, generate a random one and warn — sessions
	// will not survive a restart.
	sessionSecret := []byte(os.Getenv("ISLEY_SESSION_SECRET"))
	if len(sessionSecret) == 0 {
		logger.Log.Warn("ISLEY_SESSION_SECRET not set; generating a random session key (sessions will not survive restart)")
		sessionSecret = make([]byte, 32)
		if _, err := rand.Read(sessionSecret); err != nil {
			logger.Log.WithError(err).Fatal("Failed to generate random session key")
		}
	}

	if os.Getenv("GIN_MODE") == "" {
		// Default to release mode in production; override with GIN_MODE=debug.
		gin.SetMode(gin.ReleaseMode)
	}

	engine, err := app.NewEngine(app.Config{
		DB:             db,
		Assets:         embeddedFiles,
		Version:        version,
		SessionSecret:  sessionSecret,
		SecureCookies:  handlers.SecureCookies,
		GuestMode:      config.GuestMode == 1,
		TrustedProxies: trustedProxies,
	})
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to construct HTTP engine")
	}

	bgWG.Add(1)
	go func() {
		defer bgWG.Done()
		watcher.Grab(ctx)
	}()

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: engine,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.WithError(err).Fatal("HTTP server error")
		}
	}()

	logger.Log.WithField("port", port).Info("Server started")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info("Shutdown signal received, stopping gracefully...")

	cancel()
	bgWG.Wait()
	logger.Log.Info("Background goroutines stopped")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Log.WithError(err).Error("HTTP server forced to shutdown")
	}

	logger.Log.Info("Server exited cleanly")
}

func getVersion() string {
	data, err := embeddedFiles.ReadFile("VERSION")
	if err != nil {
		return "dev"
	}
	return strings.TrimSpace(string(data))
}
