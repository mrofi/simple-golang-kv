package main

import (
	"log"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/mrofi/simple-golang-kv/src/handlers"
	"github.com/mrofi/simple-golang-kv/src/routes"
	"github.com/mrofi/simple-golang-kv/src/store"
)

func main() {
	e := echo.New()

	// etcd endpoints, can be set via env or hardcoded for local dev
	endpoints := []string{"localhost:2379"}
	if envEndpoints := os.Getenv("ETCD_ENDPOINTS"); envEndpoints != "" {
		endpoints = []string{envEndpoints}
	}

	// Read TLS certs from environment variables if provided
	caFile := os.Getenv("ETCD_CA_FILE")
	certFile := os.Getenv("ETCD_CERT_FILE")
	keyFile := os.Getenv("ETCD_KEY_FILE")

	kvStore, err := store.NewStore(endpoints, caFile, certFile, keyFile)
	if err != nil {
		log.Fatalf("Failed to connect to etcd: %v", err)
	}
	defer kvStore.Close()

	handler := &handlers.Handler{Store: kvStore}

	routes.SetupRoutes(e, handler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on :%s\n", port)
	if err := e.Start(":" + port); err != nil {
		log.Fatal("Error starting server: ", err)
	}
}
