package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"
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

// Webhook represents a stored webhook
type Webhook struct {
	ID           string            `json:"id"`
	Namespace    string            `json:"namespace"` // Namespace
	AppName      string            `json:"appName"`   // App name
	Key          string            `json:"key"`       // Key pattern
	Event        string            `json:"event"`     // Event type
	Endpoint     string            `json:"endpoint"`  // Webhook URL
	Method       string            `json:"method"`    // HTTP method to use
	Headers      map[string]string `json:"headers,omitempty"`
	Payload      any               `json:"payload,omitempty"`
	AddEventData bool              `json:"add_event_data"`    // Add event data to the payload
	Once         bool              `json:"once"`              // If true, webhook will be deleted after first trigger
	Timeout      int               `json:"timeout,omitempty"` // Timeout in seconds
	CreatedAt    int64             `json:"created_at"`
}

// WebhookRegistration represents a webhook registration request
type WebhookRegistration struct {
	Key          string            `json:"key"`              // Key pattern (supports * suffix for prefix matching)
	Event        string            `json:"event"`            // create, update, delete or combination such as "create,update"
	Endpoint     string            `json:"endpoint"`         // URL where webhook should be sent
	Method       string            `json:"method,omitempty"` // HTTP method to use
	Headers      map[string]string `json:"headers,omitempty"`
	Payload      any               `json:"payload,omitempty"`
	AddEventData bool              `json:"add_event_data,omitempty"` // Add event data to the payload
	Once         bool              `json:"once,omitempty"`           // If true, webhook will be deleted after first trigger
	Timeout      int               `json:"timeout,omitempty"`        // Timeout in seconds
}

// WebhookUpdate represents an update request for a webhook
type WebhookUpdate struct {
	Key          string            `json:"key,omitempty"`
	Event        string            `json:"event,omitempty"`
	Endpoint     string            `json:"endpoint,omitempty"`
	Method       string            `json:"method,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Payload      any               `json:"payload,omitempty"`
	AddEventData bool              `json:"add_event_data,omitempty"`
	Once         bool              `json:"once,omitempty"`
	Timeout      int               `json:"timeout,omitempty"` // Timeout in seconds
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
	// validate namespace and app name
	namespace := h.getNamespace(c)
	appName := h.getAppName(c)
	if err := h.validateNamespaceAppName(namespace, appName); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	// get total webhooks for this namespace/app
	webhooks, err := h.Store.All(h.getWebhookPrefix(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get existing webhooks"})
	}
	if len(webhooks) >= h.Config.MaxWebhooksAllowed {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Maximum number of webhooks reached " + strconv.Itoa(h.Config.MaxWebhooksAllowed)})
	}

	var reg WebhookRegistration
	if err := c.Bind(&reg); err != nil {
		fmt.Println(err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid input"})
	}

	// Validate required fields
	if reg.Key == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Key must not be empty"})
	}
	if reg.Event == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Event must not be empty"})
	}
	// validate event
	// check if event is one of create, update, delete or combination such as "create,update"
	if strings.Contains(reg.Event, ",") {
		events := strings.Split(reg.Event, ",")
		for _, e := range events {
			e = strings.TrimSpace(e)
			if !slices.Contains([]string{string(EventCreate), string(EventUpdate), string(EventDelete)}, e) {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Event must be one of: create, update, delete or combination"})
			}
		}
	} else {
		if !slices.Contains([]string{string(EventCreate), string(EventUpdate), string(EventDelete)}, strings.ToLower(reg.Event)) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Event must be one of: create, update, delete or combination"})
		}
	}
	// Validate endpoint
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

	if reg.Timeout == 0 {
		reg.Timeout = h.Config.DefaultWebhookTimeout
	}

	// Generate unique webhook ID
	webhookID := uuid.New().String()

	// if payload is not string, convert payload to string
	var payload any
	if reg.Payload != nil {
		if json.Valid([]byte(fmt.Sprintf("%v", reg.Payload))) {
			payload = json.RawMessage([]byte(fmt.Sprintf("%v", reg.Payload)))
		} else {
			payload = reg.Payload
		}
	}

	// Create webhook object
	webhook := Webhook{
		ID:           webhookID,
		Namespace:    h.getNamespace(c),
		AppName:      h.getAppName(c),
		Key:          reg.Key,
		Event:        strings.ReplaceAll(strings.ToLower(reg.Event), " ", ""),
		Endpoint:     reg.Endpoint,
		Method:       reg.Method,
		Headers:      reg.Headers,
		Payload:      payload,
		AddEventData: reg.AddEventData,
		Once:         reg.Once,
		Timeout:      reg.Timeout,
		CreatedAt:    time.Now().Unix(),
	}

	// Store webhook
	webhookKey := h.getWebhookKey(c, webhookID)
	webhookJSON, err := json.Marshal(webhook)
	if err != nil {
		fmt.Println(err)
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
		key := c.QueryParam("key")
		if key != "" {
			// if key query param is provided, get webhooks for that key pattern
			return h.GetWebhooksForPattern(c, key)
		}
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errWebhookIDEmpty})
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

	// fmt.Println("Getting webhooks for pattern:", h.getWebhookPrefix(c), webhooks[0].Value)
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
		fmt.Println(err, webhook)
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
		// validate event
		// check if event is one of create, update, delete or combination such as "create,update"
		if strings.Contains(update.Event, ",") {
			events := strings.Split(update.Event, ",")
			for _, e := range events {
				e = strings.TrimSpace(e)
				e = strings.ToLower(e)
				if !slices.Contains([]string{string(EventCreate), string(EventUpdate), string(EventDelete)}, e) {
					return echo.NewHTTPError(http.StatusBadRequest, map[string]string{"error": "Event must be one of: create, update, delete or combination"})
				}
			}
		} else {
			if !slices.Contains([]string{string(EventCreate), string(EventUpdate), string(EventDelete)}, strings.ToLower(update.Event)) {
				return echo.NewHTTPError(http.StatusBadRequest, map[string]string{"error": "Event must be one of: create, update, delete or combination"})
			}
		}
		webhook.Event = strings.ReplaceAll(strings.ToLower(update.Event), " ", "")
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
		// if payload is not string, convert payload to string
		var payloadStr string
		switch v := update.Payload.(type) {
		case string:
			payloadStr = v
		default:
			payloadBytes, err := json.Marshal(v)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, map[string]string{"error": "Invalid payload"})
			}
			payloadStr = string(payloadBytes)
		}
		webhook.Payload = json.RawMessage([]byte(payloadStr))
	}
	if update.AddEventData != webhook.AddEventData {
		webhook.AddEventData = update.AddEventData
	}
	if update.Once != webhook.Once {
		webhook.Once = update.Once
	}
	if update.Timeout != 0 {
		webhook.Timeout = update.Timeout
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
		if !strings.Contains(webhook.Event, string(event)) {
			continue
		}

		// Check if key matches
		if !h.keyMatches(webhook.Key, key) {
			continue
		}

		// Trigger webhook asynchronously
		go h.sendWebhook(webhook, event, key, kvItem)
	}
}

// buildEventData builds the event data structure
func (h *Handler) buildEventData(webhook Webhook, key string, kvItem *store.KVItem) map[string]any {
	eventData := make(map[string]any)
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
func (h *Handler) sendHTTPRequest(webhook Webhook, eventData map[string]any) error {
	// set payload
	var bodyBytes []byte

	if webhook.Payload != nil {
		switch v := webhook.Payload.(type) {

		case json.RawMessage:
			bodyBytes = v

		case string:
			bodyBytes = []byte(v)

		case []byte:
			bodyBytes = v

		default:
			// marshal unknown types (maps, structs, etc.)
			b, _ := json.Marshal(v)
			bodyBytes = b
		}
	}

	// create HTTP request
	req, err := http.NewRequest(webhook.Method, webhook.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	// Set headers
	for key, value := range webhook.Headers {
		req.Header.Set(key, value)
	}

	// add event data header
	if len(eventData) > 0 {
		eventDataJSON, err := json.Marshal(eventData)
		if err == nil {
			req.Header.Set("X-Webhook-Event-Data", string(eventDataJSON))
		}
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: time.Duration(webhook.Timeout) * time.Second,
	}

	// Send HTTP request
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook request failed with status code %d", resp.StatusCode)
	}

	return nil
}

func (h *Handler) getWebhookKeyFromWebhook(webhook Webhook) string {
	return h.getWebhookKeyFromParams(webhook.Namespace, webhook.AppName, webhook.ID)
}

func (h *Handler) getWebhookKeyFromParams(namespace, appName, webhookID string) string {
	return "/" + h.Config.BaseKeyPrefix + "/webhooks/" + namespace + "/" + appName + "/" + webhookID
}

// sendWebhook sends the webhook HTTP request
func (h *Handler) sendWebhook(webhook Webhook, event WebhookEvent, key string, kvItem *store.KVItem) {
	eventData := map[string]any{}
	if webhook.AddEventData {
		eventData = h.buildEventData(webhook, key, kvItem)
	}
	fmt.Println("Triggering webhook:", webhook.ID, ", for key:", key, ", event:", event)
	if err := h.sendHTTPRequest(webhook, eventData); err != nil {
		log.Printf("Error sending webhook for key %s to %s: %v", key, webhook.Endpoint, err)
	}
	// If webhook is once, delete it after triggering
	if webhook.Once {
		webhookKey := h.getWebhookKeyFromWebhook(webhook)
		fmt.Println("Deleting one-time webhook:", webhookKey)
		if err := h.Store.Delete(webhookKey); err != nil {
			log.Printf("Error deleting one-time webhook %s: %v", webhook.ID, err)
		}
	}
}
