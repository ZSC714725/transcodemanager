// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"

	"github.com/zkevindev/transcodemanager/internal/api"
	"github.com/zkevindev/transcodemanager/internal/config"
	"github.com/zkevindev/transcodemanager/internal/events"
	"github.com/zkevindev/transcodemanager/internal/ffmpeg"
	"github.com/zkevindev/transcodemanager/internal/logger"
	"github.com/zkevindev/transcodemanager/internal/metrics"
	"github.com/zkevindev/transcodemanager/internal/task"
)

// buildLogWriter returns the shared log destination. With no file it is stdout;
// with a file it tees to both stdout and a size-rotated file (via lumberjack),
// returning a closer for the file handle.
func buildLogWriter(cfg config.LoggingConfig) (io.Writer, func(), error) {
	if cfg.File == "" {
		return os.Stdout, func() {}, nil
	}
	if dir := filepath.Dir(cfg.File); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, err
		}
	}
	lj := &lumberjack.Logger{
		Filename:   cfg.File,
		MaxSize:    cfg.MaxSizeMB,  // MB before rotation
		MaxBackups: cfg.MaxBackups, // number of old files to keep
		MaxAge:     cfg.MaxAgeDays, // days; 0 = no age-based cleanup
		Compress:   cfg.Compress,
	}
	return io.MultiWriter(os.Stdout, lj), func() { _ = lj.Close() }, nil
}

func toAPIPresets(in []config.PresetConfig) []api.Preset {
	out := make([]api.Preset, 0, len(in))
	for _, p := range in {
		out = append(out, api.Preset{
			Name:          p.Name,
			Description:   p.Description,
			InputOptions:  p.InputOptions,
			OutputOptions: p.OutputOptions,
		})
	}
	return out
}

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

	// One shared destination for everything: the service's own logs, gin's access
	// log, gin's startup/route output, and any stray standard-library log calls.
	logOut, closeLog, err := buildLogWriter(cfg.Logging)
	if err != nil {
		log.Fatalf("open log file: %v", err)
	}
	defer closeLog()
	log.SetOutput(logOut)
	gin.DefaultWriter = logOut
	gin.DefaultErrorWriter = logOut

	appLog := logger.NewWithOptions(logger.Options{
		Prefix: "transcodemanager: ",
		Level:  logger.ParseLevel(cfg.Logging.Level),
		Format: cfg.Logging.Format,
		Output: logOut,
	})

	// Address filters constrain what ffmpeg may read/write, mitigating arbitrary
	// file access and SSRF from the task API.
	valIn, err := ffmpeg.NewValidator(cfg.FFmpeg.AllowInput, cfg.FFmpeg.BlockInput)
	if err != nil {
		log.Fatalf("invalid input address filter: %v", err)
	}
	valOut, err := ffmpeg.NewValidator(cfg.FFmpeg.AllowOutput, cfg.FFmpeg.BlockOutput)
	if err != nil {
		log.Fatalf("invalid output address filter: %v", err)
	}

	if cfg.Observability.TaskLogDir != "" {
		if err := os.MkdirAll(cfg.Observability.TaskLogDir, 0o755); err != nil {
			log.Fatalf("create task log dir: %v", err)
		}
	}

	ff, err := ffmpeg.New(ffmpeg.Config{
		Binary:          ffmpegPath,
		MaxLogLines:     cfg.Observability.MaxLogLines,
		ValidatorInput:  valIn,
		ValidatorOutput: valOut,
		TaskLogDir:      cfg.Observability.TaskLogDir,
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
		PersistPath:   cfg.Observability.PersistPath,
		MaxConcurrent: cfg.Server.MaxConcurrentTasks,
	})
	if m != nil {
		m.BindStore(store)
	}

	handler := api.NewHandler(store, ff, appLog, dispatcher)
	handler.SetPresets(toAPIPresets(cfg.Presets))

	// SIGHUP hot-reloads the safely-mutable settings from the config file:
	// log level, presets, and the concurrency cap. (bind/gin_mode/log file still
	// require a restart.)
	if *configPath != "" {
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)
		go func() {
			for range hup {
				newCfg, err := config.Load(*configPath)
				if err != nil {
					appLog.Error("reload config: %v", err)
					continue
				}
				appLog.SetLevel(logger.ParseLevel(newCfg.Logging.Level))
				store.SetMaxConcurrent(newCfg.Server.MaxConcurrentTasks)
				handler.SetPresets(toAPIPresets(newCfg.Presets))
				appLog.Info("config reloaded (log level, presets, max_concurrent_tasks)")
			}
		}()
	}

	gin.SetMode(cfg.Server.GinMode)
	// gin.New() (not Default) so Logger and Recovery are attached exactly once.
	r := gin.New()
	middlewares := []gin.HandlerFunc{
		gin.Logger(),
		gin.Recovery(),
		api.CORSMiddleware(cfg.Server.CORS),
		api.RateLimit(cfg.Server.RateLimit, cfg.Server.RateLimitBurst), // before auth, to blunt key brute-force
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

		v3.GET("/system/stats", handler.SystemStats)
		v3.GET("/presets", handler.ListPresets)

		v3.GET("/process/summary", handler.ProcessSummary)
		v3.GET("/process", handler.ListProcesses)
		v3.POST("/process", handler.AddProcess)
		v3.POST("/process/batch", handler.BatchCommand)
		v3.DELETE("/process/cleanup", handler.Cleanup)
		v3.GET("/process/:id", handler.GetProcess)
		v3.PUT("/process/:id", handler.UpdateProcess)
		v3.DELETE("/process/:id", handler.DeleteProcess)
		v3.GET("/process/:id/config", handler.GetConfig)
		v3.GET("/process/:id/state", handler.GetState)
		v3.GET("/process/:id/report", handler.GetReport)
		v3.GET("/process/:id/report/download", handler.DownloadReport)
		v3.PUT("/process/:id/command", handler.Command)
	}

	// baseCtx is the parent of every request context. Cancelling it on shutdown
	// unblocks long-lived handlers (the SSE event stream) so Shutdown can drain
	// immediately instead of waiting out its timeout.
	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()

	srv := &http.Server{
		Addr:        bindAddr,
		Handler:     r,
		BaseContext: func(net.Listener) context.Context { return baseCtx },
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

	// End in-flight streaming handlers (SSE) before draining connections.
	cancelBase()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		appLog.Error("shutdown: %v", err)
	}
	appLog.Info("stopped")
}
