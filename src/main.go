package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/mrofi/simple-golang-kv/src/config"
	"github.com/mrofi/simple-golang-kv/src/handlers"
	"github.com/mrofi/simple-golang-kv/src/routes"
	"github.com/mrofi/simple-golang-kv/src/store"
)

func main() {
	e := echo.New()
	e.HideBanner = true

	store, err := store.NewStore()
	if err != nil {
		log.Fatalf("Failed to connect to etcd: %v", err)
	}
	defer store.Close()

	handler := handlers.NewHandler(store)
	routes.SetupRoutes(e, handler)

	// Start watcher in background (only one pod will acquire the lock)
	watcherCtx, watcherCancel := context.WithCancel(context.Background())
	defer watcherCancel()
	go handler.StartWatcher(watcherCtx)

	// Start server in a goroutine
	go func() {
		if err := e.Start(":" + config.AppConfig.Port); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal("shutting down the server:", err)
		}
	}()

	// Create a channel to listen for OS signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit // Block until a signal is received

	log.Println("Shutting down...")

	// Step 1: Stop the watcher first (it's a background process)
	watcherCancel()
	// Give watcher a moment to clean up (unlock and close session)
	time.Sleep(2000 * time.Millisecond)
	log.Println("Watcher stopped")

	// Step 2: Shutdown Echo server (stop accepting new requests, wait for in-flight)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		e.Logger.Fatal(err)
	}
	log.Println("Echo server stopped")

	// Step 3: Close store connection last (everything depends on it)
	// Store session uses background context, so it should close cleanly
	if err := store.Close(); err != nil {
		log.Printf("Error closing store: %v", err)
	}
	log.Println("Store closed")

	log.Println("Server gracefully shut down.")
}
