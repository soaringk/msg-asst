package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/soaringk/wechat-meeting-scribe/entity/config"
	"github.com/soaringk/wechat-meeting-scribe/logic/bot"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	selectRooms := flag.Bool("select-rooms", false, "Interactive room selection mode")
	flag.Parse()

	if err := config.Load(); err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	b := bot.New()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("\n\n🛑 Received %v, shutting down gracefully...", sig)
		b.Stop()
		os.Exit(0)
	}()

	if err := b.Start(*selectRooms); err != nil {
		log.Fatalf("Fatal error: %v", err)
	}
	b.Stop()
}
