package handlers

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/mrofi/simple-golang-kv/src/store"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

const (
	webhookPathSegment = "/webhooks/"
	errUnlockWatcher   = "Error unlocking watcher: %v"
)

// StartWatcher starts a watcher that monitors all KV changes and triggers webhooks.
// Only one pod can run the watcher at a time (enforced by distributed lock).
// If the watcher pod crashes, the lock will expire (TTL 10s) and another pod will take over.
func (h *Handler) StartWatcher(ctx context.Context) {
	lockKey := "/" + h.Config.BaseKeyPrefix + "/locks/watcher"

	// Retry loop: keep trying to acquire the lock until successful or context is canceled
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Try to acquire the lock
			if h.tryAcquireLockAndWatch(ctx, lockKey) {
				time.Sleep(2 * time.Second) // Wait a bit before retrying
			} else {
				time.Sleep(5 * time.Second)
			}
		}
	}
}

// tryAcquireLockAndWatch attempts to acquire the lock and start watching.
// Returns true if lock was acquired and watcher started, false otherwise.
func (h *Handler) tryAcquireLockAndWatch(ctx context.Context, lockKey string) bool {
	// Create a separate session for the watcher lock with a context that won't be canceled
	sessionCtx := context.Background()
	watcherSession, err := concurrency.NewSession(h.Store.Client(), concurrency.WithTTL(10), concurrency.WithContext(sessionCtx))
	if err != nil {
		log.Printf("Failed to create watcher session: %v", err)
		return false
	}
	defer watcherSession.Close()

	// Use distributed lock to ensure only one pod runs the watcher
	mu := concurrency.NewMutex(watcherSession, lockKey)

	// Try to acquire the lock with a timeout
	lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := mu.Lock(lockCtx); err != nil {
		return false
	}

	// Use a separate context for unlock that won't be canceled
	unlockCtx := context.Background()
	unlocked := false
	defer func() {
		if !unlocked {
			h.unlockMutex(mu, unlockCtx)
		}
	}()

	log.Println("Watcher lock acquired, starting to watch for changes...")

	// Watch all KV changes under the base prefix
	kvPrefix := "/" + h.Config.BaseKeyPrefix + "/kv/"

	// Initialize previous values by loading all existing keys
	previousValues := h.initializePreviousValues(kvPrefix)

	watchChan := h.Store.Client().Watch(ctx, kvPrefix, clientv3.WithPrefix())

	return h.watchForChanges(ctx, mu, unlockCtx, &unlocked, watcherSession, watchChan, previousValues)
}

// initializePreviousValues loads all existing KV pairs to track create vs update.
func (h *Handler) initializePreviousValues(kvPrefix string) map[string]string {
	previousValues := make(map[string]string)
	existingKVs, err := h.Store.All(kvPrefix)
	if err == nil {
		for _, kv := range existingKVs {
			// Skip webhook keys and lock keys
			if !strings.Contains(kv.Key, webhookPathSegment) && !strings.Contains(kv.Key, "/locks/") {
				previousValues[kv.Key] = kv.Value
			}
		}
		log.Printf("Initialized watcher with %d existing keys", len(previousValues))
	}
	return previousValues
}

// unlockMutex unlocks the mutex and logs any errors.
func (h *Handler) unlockMutex(mu *concurrency.Mutex, unlockCtx context.Context) {
	if err := mu.Unlock(unlockCtx); err != nil {
		log.Printf(errUnlockWatcher, err)
	}
}

// watchForChanges watches for KV changes and triggers webhooks.
func (h *Handler) watchForChanges(ctx context.Context, mu *concurrency.Mutex, unlockCtx context.Context, unlocked *bool, watcherSession *concurrency.Session, watchChan clientv3.WatchChan, previousValues map[string]string) bool {
	for {
		select {
		case <-ctx.Done():
			log.Println("Watcher context canceled, stopping...")
			if !*unlocked {
				h.unlockMutex(mu, unlockCtx)
				*unlocked = true
			}
			return true
		case <-watcherSession.Done():
			log.Println("Watcher session expired, lock will be released automatically")
			return true
		case watchResp, ok := <-watchChan:
			if !ok {
				log.Println("Watch channel closed, stopping watcher...")
				if !*unlocked {
					h.unlockMutex(mu, unlockCtx)
					*unlocked = true
				}
				return true
			}
			h.processWatchEvents(ctx, watchResp.Events, previousValues)
		}
	}
}

// processWatchEvents processes watch events and triggers webhooks.
func (h *Handler) processWatchEvents(ctx context.Context, events []*clientv3.Event, previousValues map[string]string) {
	for _, event := range events {
		key := string(event.Kv.Key)

		// Skip webhook keys and lock keys
		if strings.Contains(key, webhookPathSegment) || strings.Contains(key, "/locks/") {
			continue
		}

		eventType, kvItem := h.processWatchEvent(ctx, event, key, previousValues)
		if eventType != "" {
			h.triggerWebhooksForKey(key, eventType, kvItem)
		}
	}
}

// processWatchEvent processes a watch event and returns the event type and KV item.
func (h *Handler) processWatchEvent(ctx context.Context, event *clientv3.Event, key string, previousValues map[string]string) (WebhookEvent, *store.KVItem) {
	switch event.Type {
	case mvccpb.PUT:
		// Determine if this is create or update
		var eventType WebhookEvent
		if _, exists := previousValues[key]; exists {
			eventType = EventUpdate
		} else {
			eventType = EventCreate
		}
		// Store current value
		previousValues[key] = string(event.Kv.Value)
		// Create KVItem
		kvItem := &store.KVItem{
			Key:   key,
			Value: string(event.Kv.Value),
		}
		// Get TTL if lease exists
		if event.Kv.Lease > 0 {
			ttlResp, err := h.Store.Client().TimeToLive(ctx, clientv3.LeaseID(event.Kv.Lease))
			if err == nil && ttlResp.TTL > 0 {
				ttl := int64(ttlResp.TTL)
				kvItem.TTL = &ttl
			}
		}
		return eventType, kvItem

	case mvccpb.DELETE:
		// Get previous value before deletion
		var kvItem *store.KVItem
		if prevValue, exists := previousValues[key]; exists {
			kvItem = &store.KVItem{
				Key:   key,
				Value: prevValue,
			}
			delete(previousValues, key)
		}
		return EventDelete, kvItem

	default:
		return "", nil
	}
}
