package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/soaringk/wechat-meeting-scribe/entity/config"
	"github.com/soaringk/wechat-meeting-scribe/logic/bot"
	"github.com/soaringk/wechat-meeting-scribe/pkg/logging"
	"go.uber.org/zap"
)

func main() {
	defer logging.Sync()

	selectRooms := flag.Bool("select-rooms", false, "Interactive room selection mode")
	flag.Parse()

	if err := config.Load(); err != nil {
		logging.Fatal("Failed to load configuration", zap.Error(err))
	}

	b := bot.New()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logging.Info("Shutting down gracefully", zap.Any("signal", sig))
		b.Stop()
		os.Exit(0)
	}()

	if err := b.Start(*selectRooms); err != nil {
		logging.Fatal("Fatal error", zap.Error(err))
	}
	b.Stop()
}
