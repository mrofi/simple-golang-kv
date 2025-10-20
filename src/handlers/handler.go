package handlers

import (
	"github.com/labstack/echo/v4"
	"github.com/mrofi/simple-golang-kv/src/config"
	"github.com/mrofi/simple-golang-kv/src/store"
)

var (
	headerNamespace = config.AppConfig.HeaderNamespace
	headerAppName   = config.AppConfig.HeaderAppName
)

// Handler wraps the etcd-backed store.
type Handler struct {
	Store *store.Store
}

// getNamespace retrieves the namespace from headers or defaults.
func getNamespace(c echo.Context) string {
	namespace := c.Request().Header.Get(headerNamespace)
	if namespace == "" {
		namespace = defaultNamespace
	}
	return namespace
}

// getAppName retrieves the app name from headers or defaults.
func getAppName(c echo.Context) string {
	appName := c.Request().Header.Get(headerAppName)
	if appName == "" {
		appName = defaultAppName
	}
	return appName
}
