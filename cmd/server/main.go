// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ZSC714725/transcodemanager/internal/api"
	"github.com/ZSC714725/transcodemanager/internal/config"
	"github.com/ZSC714725/transcodemanager/internal/events"
	"github.com/ZSC714725/transcodemanager/internal/ffmpeg"
	"github.com/ZSC714725/transcodemanager/internal/logger"
	"github.com/ZSC714725/transcodemanager/internal/metrics"
	"github.com/ZSC714725/transcodemanager/internal/task"
)

func main() {
	configPath := flag.String("config", "", "Path to YAML config file")
	bind := flag.String("bind", "", "Bind address (overrides config)")
	ffmpegBin := flag.String("ffmpeg", "", "FFmpeg binary path (overrides config)")
	flag.Parse()

	cfg := config.Default()
	if *configPath != "" {
		var err error
		cfg, err = config.Load(*configPath)
		if err != nil {
			log.Fatalf("Load config: %v", err)
		}
	}

	bindAddr := cfg.Server.Bind
	if *bind != "" {
		bindAddr = *bind
	}
	ffmpegPath := cfg.FFmpeg.Path
	if *ffmpegBin != "" {
		ffmpegPath = *ffmpegBin
	}

	appLog := logger.NewWithOptions(logger.Options{
		Prefix: "transcodemanager: ",
		Level:  logger.ParseLevel(cfg.Logging.Level),
		Format: cfg.Logging.Format,
	})

	ff, err := ffmpeg.New(ffmpeg.Config{
		Binary:      ffmpegPath,
		MaxLogLines: cfg.Observability.MaxLogLines,
	})
	if err != nil {
		log.Fatalf("FFmpeg init: %v", err)
	}

	dispatcher := events.NewDispatcher(cfg.Hooks, appLog)

	var observers []task.EventObserver
	var m *metrics.Metrics
	if cfg.Observability.MetricsEnabled {
		m = metrics.New()
		observers = append(observers, m)
	}
	observers = append(observers, dispatcher)

	store := task.NewStore(ff, appLog, task.NewCompositeObserver(observers...), task.StoreOptions{
		PersistPath: cfg.Observability.PersistPath,
	})
	if m != nil {
		m.BindStore(store)
	}

	handler := api.NewHandler(store, ff, appLog, dispatcher)

	r := gin.Default()
	middlewares := []gin.HandlerFunc{
		gin.Recovery(),
		api.CORSMiddleware(cfg.Server.CORS),
		api.AuthMiddleware(cfg.Server.APIKey, cfg.Observability.MetricsPath),
		api.RequestID(),
	}
	if m != nil {
		middlewares = append(middlewares, m.GinMiddleware())
	}
	r.Use(middlewares...)

	webDir := "web"
	indexPath := filepath.Join(webDir, "index.html")
	r.GET("/", func(c *gin.Context) { c.File(indexPath) })

	r.GET("/health/live", handler.HealthLive)
	r.GET("/health/ready", handler.HealthReady)

	if cfg.Observability.MetricsEnabled {
		r.GET(cfg.Observability.MetricsPath, metrics.Handler())
	}

	v3 := r.Group("/api/v3")
	{
		v3.GET("/skills", handler.Skills)
		v3.POST("/skills/reload", handler.ReloadSkills)

		v3.GET("/hooks", handler.ListHooks)
		v3.POST("/hooks/webhook", handler.AddWebhook)
		v3.DELETE("/hooks/webhook/:id", handler.DeleteWebhook)
		v3.GET("/events/stream", handler.EventStream)

		v3.GET("/process/summary", handler.ProcessSummary)
		v3.GET("/process", handler.ListProcesses)
		v3.POST("/process", handler.AddProcess)
		v3.GET("/process/:id", handler.GetProcess)
		v3.PUT("/process/:id", handler.UpdateProcess)
		v3.DELETE("/process/:id", handler.DeleteProcess)
		v3.GET("/process/:id/config", handler.GetConfig)
		v3.GET("/process/:id/state", handler.GetState)
		v3.GET("/process/:id/report", handler.GetReport)
		v3.PUT("/process/:id/command", handler.Command)
	}

	srv := &http.Server{
		Addr:    bindAddr,
		Handler: r,
	}

	go func() {
		appLog.Info("listening on %s (Web UI: /)", bindAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	appLog.Info("shutting down...")
	store.StopAll()
	if err := store.Flush(); err != nil {
		appLog.Error("flush tasks: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		appLog.Error("shutdown: %v", err)
	}
	appLog.Info("stopped")
}
