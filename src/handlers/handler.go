package handlers

import (
	"github.com/labstack/echo/v4"
	"github.com/mrofi/simple-golang-kv/src/config"
	"github.com/mrofi/simple-golang-kv/src/store"
)

// Handler wraps the etcd-backed store.
type Handler struct {
	Config *config.Config
	Store  *store.Store
}

func NewHandler(Store *store.Store) *Handler {
	return NewHandlerWithConfig(Store, config.AppConfig)
}

func NewHandlerWithConfig(Store *store.Store, cfg *config.Config) *Handler {
	return &Handler{Store: Store, Config: cfg}
}

// getNamespace retrieves the namespace from headers or defaults.
func (h *Handler) getNamespace(c echo.Context) string {
	namespace := c.Request().Header.Get(h.Config.HeaderNamespace)
	if namespace == "" {
		namespace = h.Config.DefaultNamespace
	}
	return namespace
}

// getAppName retrieves the app name from headers or defaults.
func (h *Handler) getAppName(c echo.Context) string {
	appName := c.Request().Header.Get(h.Config.HeaderAppName)
	if appName == "" {
		appName = h.Config.DefaultAppName
	}
	return appName
}
