package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/mrofi/simple-golang-kv/src/handlers"
)

// SetupRoutes registers the key-value handlers with the Echo instance.
func SetupRoutes(e *echo.Echo, h *handlers.Handler) {
	e.POST("/kv", h.CreateKeyValue)
	e.GET("/kv/:key", h.GetKeyValue)
	e.PUT("/kv/:key", h.UpdateKeyValue)
	e.DELETE("/kv/:key", h.DeleteKeyValue)
}
