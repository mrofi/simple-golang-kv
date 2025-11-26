package handlers

import (
	"fmt"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/mrofi/simple-golang-kv/src/config"
	"github.com/mrofi/simple-golang-kv/src/store"
)

// Handler wraps the etcd-backed store.
type Handler struct {
	Config *config.Config
	Store  *store.Store
}

const allowedCharacters = "abcdefghijklmnopqrstuvwxyz0123456789-_."

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

func (h *Handler) validateNamespaceAppName(namespace, appName string) error {
	if len(namespace) > h.Config.MaxNamespaceLen {
		return fmt.Errorf("namespace too long (max %d characters)", h.Config.MaxNamespaceLen)
	}
	if len(appName) > h.Config.MaxAppNameLen {
		return fmt.Errorf("app name too long (max %d characters)", h.Config.MaxAppNameLen)
	}
	if !isValidString(namespace, allowedCharacters) {
		if isValidString(strings.ToLower(namespace), allowedCharacters) {
			return fmt.Errorf("namespace must be lowercase")
		}
		return fmt.Errorf("namespace contains invalid characters")
	}
	if !isValidString(appName, allowedCharacters) {
		if isValidString(strings.ToLower(appName), allowedCharacters) {
			return fmt.Errorf("app name must be lowercase")
		}
		return fmt.Errorf("app name contains invalid characters")
	}
	return nil
}

func isValidString(s, allowedChars string) bool {
	for _, char := range s {
		if !containsRune(allowedChars, char) {
			return false
		}
	}
	return true
}

func containsRune(s string, r rune) bool {
	for _, char := range s {
		if char == r {
			return true
		}
	}
	return false
}
