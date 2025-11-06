package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/mrofi/simple-golang-kv/src/store"
)

const (
	errKeyEmpty    = "Key must not be empty"
	errKeyNotFound = "Key not found"
)

// KeyValue represents a key-value pair for JSON binding.
type KeyValue struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	TTL      int64  `json:"ttl,omitempty"`       // TTL in seconds, optional
	ExpireAt int64  `json:"expire_at,omitempty"` // Unix timestamp, optional
}

// getKVPrefix(baseKeyPrefix, namespace, appName) string
func (h *Handler) getKVPrefix(namespace, appName string) string {
	return "/" + h.Config.BaseKeyPrefix + "/kv/" + namespace + "/" + appName + "/"
}

// getKVPrefixedKey builds a key with namespace and app-name to prevent collision.
func (h *Handler) getKVPrefixedKey(c echo.Context, key string) (string, error) {
	namespace := h.getNamespace(c)
	appName := h.getAppName(c)
	if len(namespace) > h.Config.MaxNamespaceLen {
		return "", echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Namespace too long (max %d characters)", h.Config.MaxNamespaceLen))
	}
	if len(appName) > h.Config.MaxAppNameLen {
		return "", echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("App name too long (max %d characters)", h.Config.MaxAppNameLen))
	}
	if len(key) > h.Config.MaxKeyLen {
		return "", echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Key too long (max %d characters)", h.Config.MaxKeyLen))
	}
	return h.getKVPrefix(namespace, appName) + key, nil
}

// getOriginalKVKey
func (h *Handler) getOriginalKVKey(c echo.Context, prefixedKey string) (string, error) {
	namespace := h.getNamespace(c)
	appName := h.getAppName(c)
	prefix := h.getKVPrefix(namespace, appName)
	if !strings.HasPrefix(prefixedKey, prefix) {
		return "", echo.NewHTTPError(http.StatusBadRequest, "Invalid key prefix")
	}
	return strings.TrimPrefix(prefixedKey, prefix), nil
}

// CreateKeyValue handles the creation of a new key-value pair.
func (h *Handler) CreateKeyValue(c echo.Context) error {
	var kv KeyValue
	if err := c.Bind(&kv); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid input"})
	}
	if kv.Key == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errKeyEmpty})
	}
	if len(kv.Value) > h.Config.MaxValueSize {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Value too large (max %d bytes)", h.Config.MaxValueSize)})
	}
	if kv.TTL < 0 || kv.TTL > int64(h.Config.MaxTTLSeconds) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("TTL must be between 0 and %d seconds", h.Config.MaxTTLSeconds)})
	}
	// If TTL is not set, use default TTL
	if kv.TTL == 0 {
		kv.TTL = int64(h.Config.DefaultTTL)
	}
	prefixedKey, err := h.getKVPrefixedKey(c, kv.Key)
	if err != nil {
		return err
	}
	if err := h.Store.Set(prefixedKey, kv.Value, kv.TTL); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Could not create key-value pair"})
	}
	return c.JSON(http.StatusCreated, kv)
}

// fetchKVItems retrieves KV items based on prefixedKey (handles wildcard).
func (h *Handler) fetchKVItems(prefixedKey string) ([]*store.KVItem, error) {
	if strings.HasSuffix(prefixedKey, "*") {
		prefix := strings.TrimSuffix(prefixedKey, "*")
		return h.Store.All(prefix)
	}
	kvItem, found, err := h.Store.Get(prefixedKey)
	if err != nil || !found {
		return nil, err
	}
	return []*store.KVItem{kvItem}, nil
}

// buildKVResponse builds a response item from a KVItem.
func (h *Handler) buildKVResponse(c echo.Context, kv *store.KVItem) any {
	var ttl *int64
	var expireAt *int64
	if kv.TTL != nil {
		ttl = kv.TTL
		exp := time.Now().Unix() + *kv.TTL
		if *kv.TTL == 0 {
			exp = time.Now().Unix()
		}
		expireAt = &exp
	}

	key := kv.Key
	if originalKey, err := h.getOriginalKVKey(c, kv.Key); err == nil {
		key = originalKey
	}

	return struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		TTL      *int64 `json:"ttl"`
		ExpireAt *int64 `json:"expire_at"`
	}{
		Key:      key,
		Value:    kv.Value,
		TTL:      ttl,
		ExpireAt: expireAt,
	}
}

// GetKeyValue handles the retrieval of a key-value pair by key.
func (h *Handler) GetKeyValue(c echo.Context) error {
	key := c.Param("key")
	if key == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errKeyEmpty})
	}
	prefixedKey, err := h.getKVPrefixedKey(c, key)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err)
	}

	result, err := h.fetchKVItems(prefixedKey)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": errKeyNotFound})
	}

	responses := make([]any, 0, len(result))
	for _, kv := range result {
		responses = append(responses, h.buildKVResponse(c, kv))
	}

	if len(responses) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": errKeyNotFound})
	}

	if strings.HasSuffix(prefixedKey, "*") {
		return c.JSON(http.StatusOK, responses)
	}
	return c.JSON(http.StatusOK, responses[0])
}

// UpdateKeyValue handles the updating of an existing key-value pair.
func (h *Handler) UpdateKeyValue(c echo.Context) error {
	key := c.Param("key")
	if key == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errKeyEmpty})
	}
	var kv KeyValue
	if err := c.Bind(&kv); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid input"})
	}
	if len(kv.Value) > h.Config.MaxValueSize {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Value too large (max %d bytes)", h.Config.MaxValueSize)})
	}
	if kv.TTL < 0 || kv.TTL > int64(h.Config.MaxTTLSeconds) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("TTL must be between 0 and %d seconds", h.Config.MaxTTLSeconds)})
	}
	// If TTL is not set, use default TTL
	if kv.TTL == 0 {
		kv.TTL = int64(h.Config.DefaultTTL)
	}
	prefixedKey, err := h.getKVPrefixedKey(c, key)
	if err != nil {
		return err
	}
	if err := h.Store.Set(prefixedKey, kv.Value, kv.TTL); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Could not update key-value pair"})
	}
	return c.JSON(http.StatusOK, KeyValue{Key: key, Value: kv.Value, TTL: kv.TTL})
}

// DeleteKeyValue handles the deletion of a key-value pair by key.
func (h *Handler) DeleteKeyValue(c echo.Context) error {
	key := c.Param("key")
	if key == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errKeyEmpty})
	}
	prefixedKey, err := h.getKVPrefixedKey(c, key)
	if err != nil {
		return err
	}
	if err := h.Store.Delete(prefixedKey); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": errKeyNotFound})
	}
	return c.NoContent(http.StatusNoContent)
}
