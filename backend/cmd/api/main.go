package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"senti/backend/internal/analyzer"
	"senti/backend/internal/config"
	apihttp "senti/backend/internal/http"
	"senti/backend/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	repo := store.NewPostgresRepository(pool)
	rules, err := analyzer.LoadRules(cfg.ChatSkillsDir)
	if err != nil {
		logger.Error("failed to load chat-skills rules", "error", err)
		os.Exit(1)
	}

	ocr := analyzer.NewTesseractOCR(cfg.OCRLanguage, logger)
	kimi := analyzer.NewKimiClient(cfg, logger)
	service := analyzer.NewService(repo, rules, ocr, kimi, logger)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apihttp.NewServer(cfg, repo, service, logger),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("server starting", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}
