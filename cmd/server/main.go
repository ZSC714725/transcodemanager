// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package main

import (
	"flag"
	"log"
	"path/filepath"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/ZSC714725/transcodemanager/internal/api"
	"github.com/ZSC714725/transcodemanager/internal/config"
	"github.com/ZSC714725/transcodemanager/internal/ffmpeg"
	"github.com/ZSC714725/transcodemanager/internal/logger"
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

	logger := logger.New("transcodemanager")

	ff, err := ffmpeg.New(ffmpeg.Config{
		Binary:      ffmpegPath,
		MaxLogLines: 100,
	})
	if err != nil {
		log.Fatalf("FFmpeg init: %v", err)
	}

	store := task.NewStore(ff, logger)
	handler := api.NewHandler(store, ff)

	r := gin.Default()
	r.Use(gin.Recovery(), cors.Default())

	// 静态前端
	webDir := "web"
	indexPath := filepath.Join(webDir, "index.html")
	r.GET("/", func(c *gin.Context) { c.File(indexPath) })

	v3 := r.Group("/api/v3")
	{
		v3.GET("/skills", handler.Skills)
		v3.POST("/skills/reload", handler.ReloadSkills)

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

	log.Printf("TranscodeManager listening on %s (Web UI: /)", bindAddr)
	if err := r.Run(bindAddr); err != nil {
		log.Fatalf("Server: %v", err)
	}
}
