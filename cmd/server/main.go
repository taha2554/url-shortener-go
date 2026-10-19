package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/taha2554/url-shortener-go/internal/shortener"
	"github.com/taha2554/url-shortener-go/internal/store"
)

func main() {
	addr := flag.String("addr", envOr("ADDR", ":8080"), "listen address")
	baseURL := flag.String("base-url", envOr("BASE_URL", "http://localhost:8080"), "public base URL used in short links")
	dataFile := flag.String("data", envOr("DATA_FILE", "links.json"), "JSON file for storing links (empty = memory only)")
	flag.Parse()

	var st store.Store
	if *dataFile == "" {
		st = store.NewMemory()
	} else {
		fileStore, err := store.NewFile(*dataFile)
		if err != nil {
			log.Fatalf("open store: %v", err)
		}
		st = fileStore
	}

	limiter := shortener.NewRateLimiter(30, 10)
	go func() {
		for range time.Tick(10 * time.Minute) {
			limiter.Cleanup(time.Hour)
		}
	}()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           (&shortener.Server{Store: st, BaseURL: *baseURL, Limiter: limiter}).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
