package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/mrofi/simple-golang-kv/src/store"
)

// WebhookEvent represents the type of event that triggers a webhook
type WebhookEvent string

const (
	EventCreate WebhookEvent = "create"
	EventUpdate WebhookEvent = "update"
	EventDelete WebhookEvent = "delete"
)

const (
	errWebhookIDEmpty  = "Webhook ID must not be empty"
	errWebhookNotFound = "Webhook not found"
)

var validMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
var defaultMethod = "POST"

// WebhookRegistration represents a webhook registration request
type WebhookRegistration struct {
	Key          string                 `json:"key"`              // Key pattern (supports * suffix for prefix matching)
	Event        string                 `json:"event"`            // create, update, or delete
	Endpoint     string                 `json:"endpoint"`         // URL where webhook should be sent
	Method       string                 `json:"method,omitempty"` // HTTP method to use
	Headers      map[string]string      `json:"headers,omitempty"`
	Payload      map[string]interface{} `json:"payload,omitempty"`
	AddEventData bool                   `json:"add_event_data,omitempty"` // Add event data to the payload
}

// Webhook represents a stored webhook
type Webhook struct {
	ID           string                 `json:"id"`
	Namespace    string                 `json:"namespace"` // Namespace
	AppName      string                 `json:"appName"`   // App name
	Key          string                 `json:"key"`       // Key pattern
	Event        string                 `json:"event"`     // Event type
	Endpoint     string                 `json:"endpoint"`  // Webhook URL
	Method       string                 `json:"method"`    // HTTP method to use
	Headers      map[string]string      `json:"headers,omitempty"`
	Payload      map[string]interface{} `json:"payload,omitempty"`
	AddEventData bool                   `json:"add_event_data"` // Add event data to the payload
	CreatedAt    int64                  `json:"created_at"`
}

// WebhookUpdate represents an update request for a webhook
type WebhookUpdate struct {
	Key          string                 `json:"key,omitempty"`
	Event        string                 `json:"event,omitempty"`
	Endpoint     string                 `json:"endpoint,omitempty"`
	Method       string                 `json:"method,omitempty"`
	Headers      map[string]string      `json:"headers,omitempty"`
	Payload      map[string]interface{} `json:"payload,omitempty"`
	AddEventData bool                   `json:"add_event_data,omitempty"`
}

// getWebhookPrefix returns the prefix for webhook storage
func (h *Handler) getWebhookPrefix(c echo.Context) string {
	namespace := h.getNamespace(c)
	appName := h.getAppName(c)
	return "/" + h.Config.BaseKeyPrefix + "/webhooks/" + namespace + "/" + appName + "/"
}

// getWebhookKey builds a prefixed key for webhook storage
func (h *Handler) getWebhookKey(c echo.Context, webhookID string) string {
	return h.getWebhookPrefix(c) + webhookID
}

// RegisterWebhook handles webhook registration
func (h *Handler) RegisterWebhook(c echo.Context) error {
	var reg WebhookRegistration
	if err := c.Bind(&reg); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid input"})
	}

	// Validate required fields
	if reg.Key == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Key must not be empty"})
	}
	if reg.Event == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Event must not be empty"})
	}
	if reg.Endpoint == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Endpoint must not be empty"})
	}

	// Validate method
	if reg.Method != "" {
		if !slices.Contains(validMethods, strings.ToUpper(reg.Method)) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid method"})
		}
	} else {
		reg.Method = defaultMethod
	}

	// Validate event type
	event := WebhookEvent(strings.ToLower(reg.Event))
	if event != EventCreate && event != EventUpdate && event != EventDelete {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Event must be one of: create, update, delete"})
	}

	// Generate unique webhook ID
	webhookID := uuid.New().String()

	// Create webhook object
	webhook := Webhook{
		ID:           webhookID,
		Namespace:    h.getNamespace(c),
		AppName:      h.getAppName(c),
		Key:          reg.Key,
		Event:        string(event),
		Endpoint:     reg.Endpoint,
		Method:       reg.Method,
		Headers:      reg.Headers,
		Payload:      reg.Payload,
		AddEventData: reg.AddEventData,
		CreatedAt:    time.Now().Unix(),
	}

	// Store webhook
	webhookKey := h.getWebhookKey(c, webhookID)
	webhookJSON, err := json.Marshal(webhook)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to serialize webhook"})
	}

	if err := h.Store.Set(webhookKey, string(webhookJSON), 0); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to register webhook"})
	}

	// Return webhook with ID
	return c.JSON(http.StatusCreated, map[string]string{"id": webhookID})
}

// GetWebhook retrieves a webhook by ID
func (h *Handler) GetWebhook(c echo.Context) error {
	webhookID := c.Param("id")
	if webhookID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errWebhookIDEmpty})
	}

	// if webhookID end with * call GetWebhooks
	if strings.HasSuffix(webhookID, "*") {
		return h.GetWebhooksForPattern(c, webhookID)
	}

	webhookKey := h.getWebhookKey(c, webhookID)
	kvItem, found, err := h.Store.Get(webhookKey)
	if err != nil || !found {
		return c.JSON(http.StatusNotFound, map[string]string{"error": errWebhookNotFound})
	}

	var webhook Webhook
	if err := json.Unmarshal([]byte(kvItem.Value), &webhook); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to parse webhook"})
	}

	return c.JSON(http.StatusOK, webhook)
}

// GetWebhooksForPattern retrieves all webhooks for a pattern
func (h *Handler) GetWebhooksForPattern(c echo.Context, pattern string) error {
	webhooks, err := h.Store.All(h.getWebhookPrefix(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get webhooks for pattern"})
	}

	responses := make([]Webhook, 0, len(webhooks))
	for _, kvItem := range webhooks {
		var webhook Webhook
		if err := json.Unmarshal([]byte(kvItem.Value), &webhook); err != nil {
			continue
		}
		if !h.keyMatches(pattern, webhook.Key) {
			continue
		}
		responses = append(responses, webhook)
	}

	return c.JSON(http.StatusOK, responses)
}

// UpdateWebhook updates an existing webhook
func (h *Handler) UpdateWebhook(c echo.Context) error {
	webhookID := c.Param("id")
	if webhookID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errWebhookIDEmpty})
	}

	var update WebhookUpdate
	if err := c.Bind(&update); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid input"})
	}

	// Get existing webhook
	webhookKey := h.getWebhookKey(c, webhookID)
	kvItem, found, err := h.Store.Get(webhookKey)
	if err != nil || !found {
		return c.JSON(http.StatusNotFound, map[string]string{"error": errWebhookNotFound})
	}

	var webhook Webhook
	if err := json.Unmarshal([]byte(kvItem.Value), &webhook); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to parse webhook"})
	}

	// Update fields if provided
	if err := h.applyWebhookUpdates(&webhook, &update); err != nil {
		return err
	}

	// Save updated webhook
	webhookJSON, err := json.Marshal(webhook)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to serialize webhook"})
	}

	if err := h.Store.Set(webhookKey, string(webhookJSON), 0); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update webhook"})
	}

	return c.JSON(http.StatusOK, webhook)
}

// DeleteWebhook deletes a webhook by ID
func (h *Handler) DeleteWebhook(c echo.Context) error {
	webhookID := c.Param("id")
	if webhookID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errWebhookIDEmpty})
	}

	webhookKey := h.getWebhookKey(c, webhookID)
	if err := h.Store.Delete(webhookKey); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": errWebhookNotFound})
	}

	return c.NoContent(http.StatusNoContent)
}

// applyWebhookUpdates applies update fields to a webhook
func (h *Handler) applyWebhookUpdates(webhook *Webhook, update *WebhookUpdate) error {
	if update.Key != "" {
		webhook.Key = update.Key
	}
	if update.Event != "" {
		event := WebhookEvent(strings.ToLower(update.Event))
		if event != EventCreate && event != EventUpdate && event != EventDelete {
			return echo.NewHTTPError(http.StatusBadRequest, "Event must be one of: create, update, delete")
		}
		webhook.Event = string(event)
	}
	if update.Endpoint != "" {
		webhook.Endpoint = update.Endpoint
	}
	if update.Method != "" {
		if !slices.Contains(validMethods, strings.ToUpper(update.Method)) {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid method")
		}
		webhook.Method = update.Method
	}
	if update.Headers != nil {
		webhook.Headers = update.Headers
	}
	if update.Payload != nil {
		webhook.Payload = update.Payload
	}
	if update.AddEventData != webhook.AddEventData {
		webhook.AddEventData = update.AddEventData
	}
	return nil
}

// keyMatches checks if a key matches a webhook pattern
func (h *Handler) keyMatches(pattern, key string) bool {
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(key, prefix)
	}
	return pattern == key
}

// slicePrefixedKey extracts namespace, app name, and key from a prefixed key
// Key format: /{basePrefix}/kv/{namespace}/{app}/{key}
func (h *Handler) slicePrefixedKey(prefixedKey string) (namespace, appName, key string) {
	parts := strings.Split(strings.TrimPrefix(prefixedKey, "/"+h.Config.BaseKeyPrefix+"/kv/"), "/")
	if len(parts) < 2 {
		return "", "", "" // Invalid key format
	}
	namespace = parts[0]
	appName = parts[1]
	key = parts[2]

	return namespace, appName, key
}

// triggerWebhooksForKey triggers webhooks for a given key and event type.
func (h *Handler) triggerWebhooksForKey(prefixedKey string, event WebhookEvent, kvItem *store.KVItem) {
	namespace, appName, key := h.slicePrefixedKey(prefixedKey)
	if namespace == "" || appName == "" {
		// Invalid key format, silently fail
		return
	}

	// Build webhook prefix
	webhookPrefix := "/" + h.Config.BaseKeyPrefix + "/webhooks/" + namespace + "/" + appName + "/"

	// Get all webhooks for this namespace/app
	allWebhooks, err := h.Store.All(webhookPrefix)
	if err != nil {
		return // Silently fail
	}

	// Filter and trigger matching webhooks
	for _, webhookKV := range allWebhooks {
		var webhook Webhook
		if err := json.Unmarshal([]byte(webhookKV.Value), &webhook); err != nil {
			continue
		}

		// Check if event matches
		if WebhookEvent(webhook.Event) != event {
			continue
		}

		// Check if key matches
		if !h.keyMatches(webhook.Key, key) {
			continue
		}

		// Trigger webhook asynchronously
		go h.sendWebhook(webhook, key, kvItem)
	}
}

// buildWebhookPayload builds the webhook payload
func (h *Handler) buildWebhookPayload(webhook Webhook, key string, kvItem *store.KVItem) ([]byte, error) {
	payload := make(map[string]interface{})

	// Add custom payload fields if provided
	if webhook.Payload != nil {
		for k, v := range webhook.Payload {
			payload[k] = v
		}
	}

	if webhook.AddEventData {
		eventData := h.buildEventData(webhook, key, kvItem)
		payload["event"] = eventData
	}

	if len(payload) == 0 {
		return []byte{}, nil
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return payloadJSON, nil
}

// buildEventData builds the event data structure
func (h *Handler) buildEventData(webhook Webhook, key string, kvItem *store.KVItem) map[string]interface{} {
	eventData := make(map[string]interface{})
	eventData["event"] = webhook.Event
	eventData["namespace"] = webhook.Namespace
	eventData["appName"] = webhook.AppName
	eventData["key"] = key
	eventData["timestamp"] = time.Now().Unix()

	if kvItem != nil {
		eventData["value"] = kvItem.Value
		if kvItem.TTL != nil {
			eventData["ttl"] = *kvItem.TTL
			eventData["expire_at"] = time.Now().Add(time.Duration(*kvItem.TTL) * time.Second).Unix()
		}
	} else {
		eventData["value"] = nil
	}

	return eventData
}

// sendHTTPRequest sends the HTTP request for a webhook
func (h *Handler) sendHTTPRequest(webhook Webhook, payloadJSON []byte) error {
	req, err := http.NewRequest(webhook.Method, webhook.Endpoint, bytes.NewBuffer(payloadJSON))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "github.com/mrofi/simple-golang-kv")
	if webhook.Headers != nil {
		for k, v := range webhook.Headers {
			req.Header.Set(k, v)
		}
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// sendWebhook sends the webhook HTTP request
func (h *Handler) sendWebhook(webhook Webhook, key string, kvItem *store.KVItem) {
	payloadJSON, err := h.buildWebhookPayload(webhook, key, kvItem)
	if err != nil {
		log.Printf("Error building payload for key %s to %s: %v", key, webhook.Endpoint, err)
		return
	}

	if err := h.sendHTTPRequest(webhook, payloadJSON); err != nil {
		log.Printf("Error sending webhook for key %s to %s: %v", key, webhook.Endpoint, err)
		return
	}
}
