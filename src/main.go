package main

import (
	"log"

	"github.com/labstack/echo/v4"
	"github.com/mrofi/simple-golang-kv/src/config"
	"github.com/mrofi/simple-golang-kv/src/handlers"
	"github.com/mrofi/simple-golang-kv/src/routes"
	"github.com/mrofi/simple-golang-kv/src/store"
)

func main() {
	e := echo.New()
	e.HideBanner = true

	endpoints := config.AppConfig.ETCDEndpoints

	// Read TLS certs from environment variables if provided
	caFile := config.AppConfig.ETCDCAFile
	certFile := config.AppConfig.ETCDCertFile
	keyFile := config.AppConfig.ETCDKeyFile

	kvStore, err := store.NewStore(endpoints, caFile, certFile, keyFile)
	if err != nil {
		log.Fatalf("Failed to connect to etcd: %v", err)
	}
	defer kvStore.Close()

	handler := &handlers.Handler{Store: kvStore}
	routes.SetupRoutes(e, handler)

	port := config.AppConfig.Port
	log.Printf("Starting server on :%s\n", port)
	if err := e.Start(":" + port); err != nil {
		log.Fatal("Error starting server: ", err)
	}
}
