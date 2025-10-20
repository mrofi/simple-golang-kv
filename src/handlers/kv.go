package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/mrofi/simple-golang-kv/src/config"
	"github.com/mrofi/simple-golang-kv/src/store"
)

var (
	baseKeyPrefix    = config.AppConfig.BaseKeyPrefix
	defaultNamespace = config.AppConfig.DefaultNamespace
	defaultAppName   = config.AppConfig.DefaultAppName
	defaultTTL       = config.AppConfig.DefaultTTL
	maxNamespaceLen  = config.AppConfig.MaxNamespaceLen
	maxAppNameLen    = config.AppConfig.MaxAppNameLen
	maxKeyLen        = config.AppConfig.MaxKeyLen
	maxValueSize     = config.AppConfig.MaxValueSize
	maxTTLSeconds    = config.AppConfig.MaxTTLSeconds
)

// KeyValue represents a key-value pair for JSON binding.
type KeyValue struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	TTL      int64  `json:"ttl,omitempty"`       // TTL in seconds, optional
	ExpireAt int64  `json:"expire_at,omitempty"` // Unix timestamp, optional
}

// getKVPrefix(baseKeyPrefix, namespace, appName) string
func getKVPrefix(baseKeyPrefix string, namespace string, appName string) string {
	return "/" + baseKeyPrefix + "/kv/" + namespace + "/" + appName + "/"
}

// getKVPrefixedKey builds a key with namespace and app-name to prevent collision.
func getKVPrefixedKey(c echo.Context, key string) (string, error) {
	namespace := getNamespace(c)
	appName := getAppName(c)
	if len(namespace) > maxNamespaceLen {
		return "", echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Namespace too long (max %d characters)", maxNamespaceLen))
	}
	if len(appName) > maxAppNameLen {
		return "", echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("App name too long (max %d characters)", maxAppNameLen))
	}
	if len(key) > maxKeyLen {
		return "", echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Key too long (max %d characters)", maxKeyLen))
	}
	return getKVPrefix(baseKeyPrefix, namespace, appName) + key, nil
}

// getOriginalKVKey
func getOriginalKVKey(c echo.Context, prefixedKey string) (string, error) {
	namespace := getNamespace(c)
	appName := getAppName(c)
	prefix := getKVPrefix(baseKeyPrefix, namespace, appName)
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
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Key must not be empty"})
	}
	if len(kv.Value) > maxValueSize {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Value too large (max %d bytes)", maxValueSize)})
	}
	if kv.TTL < 0 || kv.TTL > int64(maxTTLSeconds) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("TTL must be between 0 and %d seconds", maxTTLSeconds)})
	}
	// If TTL is not set, use default TTL
	if kv.TTL == 0 {
		kv.TTL = int64(defaultTTL)
	}
	prefixedKey, err := getKVPrefixedKey(c, kv.Key)
	if err != nil {
		return err
	}
	if err := h.Store.Set(prefixedKey, kv.Value, kv.TTL); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Could not create key-value pair"})
	}
	return c.JSON(http.StatusCreated, kv)
}

// GetKeyValue handles the retrieval of a key-value pair by key.
func (h *Handler) GetKeyValue(c echo.Context) error {
	key := c.Param("key")
	if key == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Key must not be empty"})
	}
	prefixedKey, err := getKVPrefixedKey(c, key)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err)
	}

	var result []*store.KVItem
	if strings.HasSuffix(prefixedKey, "*") {
		prefix := strings.TrimSuffix(prefixedKey, "*")
		result, err = h.Store.All(prefix)
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Key not found"})
		}
	} else {
		kvItem, found, err := h.Store.Get(prefixedKey)
		if err != nil || !found {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Key not found"})
		}
		result = []*store.KVItem{kvItem}
	}

	var responses []any
	for _, kv := range result {
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
		if originalKey, err := getOriginalKVKey(c, kv.Key); err == nil {
			key = originalKey
		}

		resp := struct {
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

		responses = append(responses, resp)
	}

	if len(responses) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Key not found"})
	}
	// Multiple items response for wildcard
	if strings.HasSuffix(prefixedKey, "*") {
		return c.JSON(http.StatusOK, responses)
	}
	// Single item response
	resp := responses[0]

	return c.JSON(http.StatusOK, resp)
}

// UpdateKeyValue handles the updating of an existing key-value pair.
func (h *Handler) UpdateKeyValue(c echo.Context) error {
	key := c.Param("key")
	if key == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Key must not be empty"})
	}
	var kv KeyValue
	if err := c.Bind(&kv); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid input"})
	}
	if len(kv.Value) > maxValueSize {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Value too large (max %d bytes)", maxValueSize)})
	}
	if kv.TTL < 0 || kv.TTL > int64(maxTTLSeconds) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("TTL must be between 0 and %d seconds", maxTTLSeconds)})
	}
	// If TTL is not set, use default TTL
	if kv.TTL == 0 {
		kv.TTL = int64(defaultTTL)
	}
	prefixedKey, err := getKVPrefixedKey(c, key)
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
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Key must not be empty"})
	}
	prefixedKey, err := getKVPrefixedKey(c, key)
	if err != nil {
		return err
	}
	if err := h.Store.Delete(prefixedKey); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Key not found"})
	}
	return c.NoContent(http.StatusNoContent)
}
