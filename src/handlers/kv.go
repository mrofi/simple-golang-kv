package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/mrofi/simple-golang-kv/src/store"
)

var (
	baseKeyPrefix    = getEnv("BASE_KEY_PREFIX", "kvstore")
	defaultNamespace = getEnv("DEFAULT_NAMESPACE", "default")
	defaultAppName   = getEnv("DEFAULT_APPNAME", "default")
	defaultTTL       = getEnvInt("DEFAULT_TTL_SECONDS", 0) // 0 means no expiration
	maxNamespaceLen  = getEnvInt("MAX_NAMESPACE_LEN", 25)
	maxAppNameLen    = getEnvInt("MAX_APPNAME_LEN", 25)
	maxKeyLen        = getEnvInt("MAX_KEY_LEN", 100)
	maxValueSize     = getEnvInt("MAX_VALUE_SIZE", 1*1024*1024)   // 1 MB
	maxTTLSeconds    = getEnvInt("MAX_TTL_SECONDS", 365*24*60*60) // 1 year
)

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return fallback
}

// KeyValue represents a key-value pair for JSON binding.
type KeyValue struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	TTL      int64  `json:"ttl,omitempty"`       // TTL in seconds, optional
	ExpireAt int64  `json:"expire_at,omitempty"` // Unix timestamp, optional
}

// Handler wraps the etcd-backed store.
type Handler struct {
	Store *store.Store
}

// getPrefixedKey builds a key with namespace and app-name to prevent collision.
func getPrefixedKey(c echo.Context, key string) (string, error) {
	namespace := c.Request().Header.Get("KV-Namespace")
	appName := c.Request().Header.Get("KV-App-Name")
	if namespace == "" {
		namespace = defaultNamespace
	}
	if appName == "" {
		appName = defaultAppName
	}
	if len(namespace) > maxNamespaceLen {
		return "", echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Namespace too long (max %d characters)", maxNamespaceLen))
	}
	if len(appName) > maxAppNameLen {
		return "", echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("App name too long (max %d characters)", maxAppNameLen))
	}
	if len(key) > maxKeyLen {
		return "", echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Key too long (max %d characters)", maxKeyLen))
	}
	return "/" + baseKeyPrefix + "/" + namespace + "/" + appName + "/" + key, nil
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
	prefixedKey, err := getPrefixedKey(c, kv.Key)
	if err != nil {
		return err
	}
	if err := h.Store.SetWithTTL(prefixedKey, kv.Value, kv.TTL); err != nil {
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
	prefixedKey, err := getPrefixedKey(c, key)
	if err != nil {
		return err
	}
	value, found, ttlPtr, err := h.Store.GetWithTTL(prefixedKey)
	if err != nil || !found {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Key not found"})
	}

	var ttl *int64
	var expireAt *int64
	if ttlPtr != nil {
		ttl = ttlPtr
		exp := time.Now().Unix() + *ttlPtr
		if *ttlPtr == 0 {
			exp = time.Now().Unix()
		}
		expireAt = &exp
	} else {
		ttl = nil
		expireAt = nil
	}

	resp := struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		TTL      *int64 `json:"ttl"`
		ExpireAt *int64 `json:"expire_at"`
	}{
		Key:      key,
		Value:    value,
		TTL:      ttl,
		ExpireAt: expireAt,
	}

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
	prefixedKey, err := getPrefixedKey(c, key)
	if err != nil {
		return err
	}
	if err := h.Store.SetWithTTL(prefixedKey, kv.Value, kv.TTL); err != nil {
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
	prefixedKey, err := getPrefixedKey(c, key)
	if err != nil {
		return err
	}
	if err := h.Store.Delete(prefixedKey); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Key not found"})
	}
	return c.NoContent(http.StatusNoContent)
}
