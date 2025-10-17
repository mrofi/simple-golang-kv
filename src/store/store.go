package store

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log"
	"os"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// Store represents a key-value store backed by etcd.
type Store struct {
	client *clientv3.Client
}

// NewStore creates a new instance of Store connected to etcd with optional TLS.
func NewStore(endpoints []string, caFile, certFile, keyFile string) (*Store, error) {
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

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
		TLS:         tlsConfig,
	})
	if err != nil {
		return nil, err
	}
	return &Store{client: cli}, nil
}

// SetWithTTL adds or updates a key-value pair in etcd with optional TTL (in seconds).
func (s *Store) SetWithTTL(key string, value string, ttl int64) error {
	ctx := context.Background()
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

// Set adds or updates a key-value pair in etcd (no TTL).
func (s *Store) Set(key string, value string) error {
	return s.SetWithTTL(key, value, 0)
}

// Get retrieves the value for a given key from etcd.
func (s *Store) Get(key string) (string, bool, error) {
	resp, err := s.client.Get(context.Background(), key)
	if err != nil || len(resp.Kvs) == 0 {
		return "", false, err
	}
	return string(resp.Kvs[0].Value), true, nil
}

// Get retrieves the value for a given key from etcd and returns its lease ID and TTL if set.
func (s *Store) GetWithTTL(key string) (value string, found bool, ttl *int64, err error) {
	resp, err := s.client.Get(context.Background(), key)
	if err != nil || len(resp.Kvs) == 0 {
		return "", false, nil, err
	}
	kv := resp.Kvs[0]
	value = string(kv.Value)
	if kv.Lease == 0 {
		return value, true, nil, nil
	}
	// Query lease TTL
	leaseResp, err := s.client.TimeToLive(context.Background(), clientv3.LeaseID(kv.Lease))
	if err != nil {
		return value, true, nil, nil // Return value even if TTL lookup fails
	}
	return value, true, &leaseResp.TTL, nil
}

// Delete removes a key-value pair from etcd.
func (s *Store) Delete(key string) error {
	_, err := s.client.Delete(context.Background(), key)
	return err
}

// All returns all key-value pairs in etcd (under a prefix).
func (s *Store) All(prefix string) (map[string]string, error) {
	resp, err := s.client.Get(context.Background(), prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, kv := range resp.Kvs {
		result[string(kv.Key)] = string(kv.Value)
	}
	return result, nil
}

// Close closes the etcd client connection.
func (s *Store) Close() error {
	return s.client.Close()
}
