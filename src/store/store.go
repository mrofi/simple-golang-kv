package store

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log"
	"os"
	"time"

	"github.com/mrofi/simple-golang-kv/src/config"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/zap"
)

// Store represents a key-value store backed by etcd.
type Store struct {
	client     *clientv3.Client
	session    *concurrency.Session
	lockPrefix string
}

type KVItem struct {
	Key   string
	Value string
	TTL   *int64 // in seconds
}

// NewStore creates a new instance of Store connected to etcd with optional TLS.
func NewStore() (*Store, error) {
	return NewStoreWithConfig(config.AppConfig)
}

// NewStoreWithConfig creates a new instance of Store connected to etcd with optional TLS.
func NewStoreWithConfig(cfg *config.Config) (*Store, error) {
	endpoints := cfg.ETCDEndpoints
	caFile := cfg.ETCDCAFile
	certFile := cfg.ETCDCertFile
	keyFile := cfg.ETCDKeyFile
	baseKeyPrefix := cfg.BaseKeyPrefix

	tlsConfig := &tls.Config{}
	if caFile != "" && certFile != "" && keyFile != "" {
		// Load CA cert
		caCert, err := os.ReadFile(caFile)
		if err != nil {
			log.Fatalf("Failed to read CA cert: %v", err)
		}

		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
			log.Fatalf("Failed to append CA cert")
		}

		// Load client cert/key pair
		clientCert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			log.Fatalf("Failed to load client cert and key: %v", err)
		}

		tlsConfig = &tls.Config{
			RootCAs:      caCertPool,
			Certificates: []tls.Certificate{clientCert},
			// ServerName: "etcd.example.com", // uncomment if needed
			MinVersion: tls.VersionTLS12,
		}
	}

	// Configure etcd client logger to suppress shutdown warnings
	// These warnings occur when the client closes while sessions are revoking leases
	zapConfig := zap.NewProductionConfig()
	zapConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel) // Only show errors, suppress warnings
	zapLogger, err := zapConfig.Build(zap.AddCallerSkip(1))
	if err != nil {
		// Fallback to default if logger creation fails
		zapLogger = zap.NewNop()
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
		TLS:         tlsConfig,
		Logger:      zapLogger,
	})
	if err != nil {
		return nil, err
	}

	// Create a session for distributed locking with a background context
	// This ensures the session's lease operations won't be affected by context cancellation
	sessionCtx := context.Background()
	session, err := concurrency.NewSession(cli, concurrency.WithTTL(10), concurrency.WithContext(sessionCtx))
	if err != nil {
		cli.Close()
		return nil, err
	}

	// Construct lock prefix using baseKeyPrefix to match the key structure
	lockPrefix := "/" + baseKeyPrefix + "/locks/"

	return &Store{
		client:     cli,
		session:    session,
		lockPrefix: lockPrefix,
	}, nil
}

// Set adds or updates a key-value pair in etcd with optional TTL (in seconds).
// This operation is protected by a distributed lock to prevent race conditions.
func (s *Store) Set(key string, value string, ttl int64) error {
	ctx := context.Background()

	// Acquire distributed lock for this key
	mu := concurrency.NewMutex(s.session, s.lockPrefix+key)
	if err := mu.Lock(ctx); err != nil {
		return err
	}
	defer mu.Unlock(ctx)

	if ttl > 0 {
		lease, err := s.client.Grant(ctx, ttl)
		if err != nil {
			return err
		}
		_, err = s.client.Put(ctx, key, value, clientv3.WithLease(lease.ID))
		return err
	}
	_, err := s.client.Put(ctx, key, value)
	return err
}

// Get retrieves the value for a given key from etcd and returns its lease ID and TTL if set.
func (s *Store) Get(key string) (kvItem *KVItem, found bool, err error) {
	resp, err := s.client.Get(context.Background(), key)
	if err != nil || len(resp.Kvs) == 0 {
		return nil, false, err
	}
	kv := s.formatKVKey(resp.Kvs[0])
	return kv, true, nil
}

// Delete removes a key-value pair from etcd.
// This operation is protected by a distributed lock to prevent race conditions.
func (s *Store) Delete(key string) error {
	ctx := context.Background()

	// Acquire distributed lock for this key
	mu := concurrency.NewMutex(s.session, s.lockPrefix+key)
	if err := mu.Lock(ctx); err != nil {
		return err
	}
	defer mu.Unlock(ctx)

	_, err := s.client.Delete(ctx, key)
	return err
}

// All returns all key-value pairs in etcd (under a prefix).
func (s *Store) All(prefix string) ([]*KVItem, error) {
	resp, err := s.client.Get(context.Background(), prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	var result []*KVItem
	for _, kv := range resp.Kvs {
		kvItem := s.formatKVKey(kv)
		result = append(result, kvItem)
	}
	return result, nil
}

// Close closes the etcd client connection and session.
func (s *Store) Close() error {
	if s.session != nil {
		s.session.Close()
	}
	return s.client.Close()
}

// Client returns the etcd client for advanced operations like watching.
func (s *Store) Client() *clientv3.Client {
	return s.client
}

// Session returns the etcd session for distributed locking.
func (s *Store) Session() *concurrency.Session {
	return s.session
}

// Formatting the KV
func (s *Store) formatKVKey(kv *mvccpb.KeyValue) *KVItem {
	formatted := &KVItem{
		Key:   string(kv.Key),
		Value: string(kv.Value),
	}
	if kv.Lease == 0 {
		return formatted
	}
	// Query lease TTL
	leaseResp, err := s.client.TimeToLive(context.Background(), clientv3.LeaseID(kv.Lease))
	if err != nil {
		return formatted // Return value even if TTL lookup fails
	}
	formatted.TTL = &leaseResp.TTL
	return formatted
}
