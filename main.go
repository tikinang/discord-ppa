package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	signer, err := NewGPGSigner(cfg.GPGPrivateKey)
	if err != nil {
		log.Fatalf("GPG error: %v", err)
	}

	s3Client := NewS3Client(cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup

	poller := NewPoller(cfg, s3Client, signer)
	wg.Add(1)
	go func() {
		defer wg.Done()
		poller.Run(ctx)
	}()

	srv := NewServer(s3Client, signer)

	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      srv.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 5 * time.Minute,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}()

	log.Printf("Listening on %s", cfg.ListenAddr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}

	wg.Wait()
	log.Println("Shutdown complete")
}
