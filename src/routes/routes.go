package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/mrofi/simple-golang-kv/src/handlers"
)

const routeKVWithKey = "/kv/:key"
const routeWebhookWithID = "/webhooks/:id"

// SetupRoutes registers the key-value handlers with the Echo instance.
func SetupRoutes(e *echo.Echo, h *handlers.Handler) {
	e.POST("/kv", h.CreateKeyValue)
	e.GET(routeKVWithKey, h.GetKeyValue)
	e.PUT(routeKVWithKey, h.UpdateKeyValue)
	e.DELETE(routeKVWithKey, h.DeleteKeyValue)

	// Webhook routes
	e.POST("/webhooks", h.RegisterWebhook)
	e.GET(routeWebhookWithID, h.GetWebhook)
	e.PUT(routeWebhookWithID, h.UpdateWebhook)
	e.DELETE(routeWebhookWithID, h.DeleteWebhook)
}
