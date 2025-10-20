package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port string

	ETCDEndpoints []string
	ETCDCAFile    string
	ETCDCertFile  string
	ETCDKeyFile   string

	BaseKeyPrefix    string
	HeaderNamespace  string
	HeaderAppName    string
	DefaultNamespace string
	DefaultAppName   string
	DefaultTTL       int
	MaxNamespaceLen  int
	MaxAppNameLen    int
	MaxKeyLen        int
	MaxValueSize     int
	MaxTTLSeconds    int
}

func NewConfig() *Config {
	return &Config{
		Port: getEnv("PORT", "8080"),

		ETCDEndpoints: []string{getEnv("ETCD_ENDPOINTS", "localhost:2379")},
		ETCDCAFile:    getEnv("ETCD_CA_FILE", ""),
		ETCDCertFile:  getEnv("ETCD_CERT_FILE", ""),
		ETCDKeyFile:   getEnv("ETCD_KEY_FILE", ""),

		BaseKeyPrefix:    getEnv("BASE_KEY_PREFIX", "kvstore"),
		HeaderNamespace:  getEnv("HEADER_NAMESPACE", "KV-Namespace"),
		HeaderAppName:    getEnv("HEADER_APPNAME", "KV-App-Name"),
		DefaultNamespace: getEnv("DEFAULT_NAMESPACE", "default"),
		DefaultAppName:   getEnv("DEFAULT_APPNAME", "default"),
		DefaultTTL:       getEnvInt("DEFAULT_TTL_SECONDS", 0), // 0 means no expiration
		MaxNamespaceLen:  getEnvInt("MAX_NAMESPACE_LEN", 25),
		MaxAppNameLen:    getEnvInt("MAX_APPNAME_LEN", 25),
		MaxKeyLen:        getEnvInt("MAX_KEY_LEN", 100),
		MaxValueSize:     getEnvInt("MAX_VALUE_SIZE", 1*1024*1024),   // 1 MB
		MaxTTLSeconds:    getEnvInt("MAX_TTL_SECONDS", 365*24*60*60), // 1 year
	}
}

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

// AppConfig is the exported configuration instance
var AppConfig = NewConfig()

// Usage example:
// import "github.com/mrofi/simple-golang-kv/src/config"
// ...
// fmt.Println(config.AppConfig.BaseKeyPrefix)
