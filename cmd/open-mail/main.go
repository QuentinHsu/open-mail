package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"open-mail/internal/config"
	"open-mail/internal/cryptox"
	"open-mail/internal/httpapi"
	"open-mail/internal/mailbox"
	"open-mail/internal/monitor"
	"open-mail/internal/store"
	"open-mail/internal/telegram"
)

func main() {
	_ = godotenv.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	fileStore, err := store.NewFileStore(cfg.DataDir)
	if err != nil {
		logger.Error("create file store", "error", err)
		os.Exit(1)
	}

	cryptoService := cryptox.NewService(cfg.MailboxEncryptionKey)
	imapClient := monitor.NewIMAPClient()
	notifier, err := telegram.NewNotifier(cfg.TelegramBotToken, cfg.TelegramAllowedChatID)
	if err != nil {
		logger.Error("create telegram notifier", "error", err)
		os.Exit(1)
	}

	mailboxService, err := mailbox.NewService(cfg, fileStore, cryptoService, imapClient, nil)
	if err != nil {
		logger.Error("create mailbox service", "error", err)
		os.Exit(1)
	}
	manager := monitor.NewManager(logger, cfg.PollInterval, imapClient, mailboxService, notifier)
	mailboxService.SetRefreshTrigger(manager)

	if err := manager.Refresh(context.Background()); err != nil {
		logger.Error("start mailbox monitors", "error", err)
		os.Exit(1)
	}

	telegramHandler := telegram.NewCommandHandler(logger, notifier, mailboxService)
	apiServer := httpapi.NewServer(mailboxService, cfg.APIToken)
	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		logger.Info("http server started", "port", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server failed", "error", err)
			cancel()
		}
	}()

	go func() {
		if err := telegramHandler.Run(ctx); err != nil && err != context.Canceled {
			logger.Error("telegram loop failed", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	manager.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown http server", "error", err)
	}
}
