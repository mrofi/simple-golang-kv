# simple-golang-kv

A simple key-value store API built with Go, Echo, and etcd.

## Features

- RESTful API for key-value operations
- Namespace and app name isolation via headers
- TTL (time-to-live) support for expiring keys
- **Webhook support** - Register webhooks to be triggered on key events (create, update, delete)
- **Automatic webhook triggering** - Background watcher automatically triggers webhooks when events occur
- **High availability** - Automatic failover: if the watcher pod crashes, another pod takes over
- etcd as backend storage
- Docker support
- Configurable port and etcd connection via environment variables
- Value size limit (default: 1MB, configurable)
- Namespace, app name, and key length limits (configurable via env)

## Usage

### Build & Run

```sh
go build -o kv-server ./src/main.go
./kv-server
```

Or use Docker:

```sh
docker build -t simple-golang-kv .
docker run -p 8080:8080 \
  -e ETCD_ENDPOINTS="localhost:2379" \
  -e ETCD_CA_FILE="/path/to/ca.crt" \
  -e ETCD_CERT_FILE="/path/to/client.crt" \
  -e ETCD_KEY_FILE="/path/to/client.key" \
  -e PORT=8080 \
  simple-golang-kv
```

### Environment Variables

- `ETCD_ENDPOINTS` — etcd endpoints (default: `localhost:2379`)
- `ETCD_CA_FILE` — CA certificate file (optional)
- `ETCD_CERT_FILE` — client certificate file (optional)
- `ETCD_KEY_FILE` — client key file (optional)
- `PORT` — HTTP port (default: `8080`)
- `BASE_KEY_PREFIX` — base key prefix (default: `kvstore`)
- `DEFAULT_NAMESPACE` — default namespace (default: `default`)
- `DEFAULT_APPNAME` — default app name (default: `default`)
- `DEFAULT_TTL_SECONDS` — default ttl in seconds (default: `0` means no expiration)
- `MAX_NAMESPACE_LEN` — max namespace length (default: `25`)
- `MAX_APPNAME_LEN` — max app name length (default: `25`)
- `MAX_KEY_LEN` — max key length (default: `100`)
- `MAX_VALUE_SIZE` — max value size in bytes (default: `1048576` for 1MB)
- `MAX_TTL_SECONDS` — max ttl in seconds (default: `31536000` for 1 year)

### API

#### Set Key

```http
POST /kv
Headers:
  KV-Namespace: myns
  KV-App-Name: myapp
Body:
{
  "key": "foo",
  "value": "bar",
  "ttl": 60
}
```

#### Get Key

```http
GET /kv/foo
Headers:
  KV-Namespace: myns
  KV-App-Name: myapp
Response:
{
  "key": "foo",
  "value": "bar",
  "ttl": 60,
  "expire_at": 1710000000
}
```

#### Update Key

```http
PUT /kv/foo
Headers:
  KV-Namespace: myns
  KV-App-Name: myapp
Body:
{
  "value": "baz",
  "ttl": 120
}
```

#### Delete Key

```http
DELETE /kv/foo
Headers:
  KV-Namespace: myns
  KV-App-Name: myapp
```

### Webhooks

Webhooks allow you to receive notifications when key-value operations occur. You can register webhooks that trigger on specific events (create, update, delete) for keys or key patterns.

#### Register Webhook

```http
POST /webhooks
Headers:
  KV-Namespace: myns
  KV-App-Name: myapp
Body:
{
  "key": "foo*",              // Key pattern (use * suffix for prefix matching)
  "event": "create",          // Event type: create, update, or delete
  "endpoint": "https://example.com/webhook",
  "method": "POST",           // Optional, default is POST (valid: GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD)
  "headers": {                // Optional custom headers
    "Authorization": "Bearer token123"
  },
  "payload": {                // Optional custom payload fields
    "source": "kv-store"
  },
  "add_event_data": true      // Optional, default false. If true, adds event data nested under "event" key
}
Response:
{
  "id": "550e8400-e29b-41d4-a716-446655440000"
}
```

#### Get Webhook

```http
GET /webhooks/{id}
Headers:
  KV-Namespace: myns
  KV-App-Name: myapp
Response:
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "namespace": "myns",
  "appName": "myapp",
  "key": "foo*",
  "event": "create",
  "endpoint": "https://example.com/webhook",
  "method": "POST",
  "headers": {
    "Authorization": "Bearer token123"
  },
  "payload": {
    "source": "kv-store"
  },
  "add_event_data": true,
  "created_at": 1710000000
}
```

#### Get Webhooks by Pattern

You can retrieve multiple webhooks by appending `*` to a key pattern in the URL:

```http
GET /webhooks/{key-pattern}*
Headers:
  KV-Namespace: myns
  KV-App-Name: myapp
Response:
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "namespace": "myns",
    "appName": "myapp",
    "key": "foo*",
    "event": "create",
    ...
  },
  ...
]
```

Note: The pattern matches against webhook keys (not IDs). For example, `GET /webhooks/foo*` returns all webhooks whose key pattern matches "foo*".

#### Update Webhook

```http
PUT /webhooks/{id}
Headers:
  KV-Namespace: myns
  KV-App-Name: myapp
Body:
{
  "key": "bar*",                    // Optional: update key pattern
  "event": "update",                // Optional: update event type
  "endpoint": "https://example.com/webhook2",  // Optional: update endpoint
  "method": "GET",                  // Optional: update method
  "headers": {                      // Optional: update headers
    "X-Custom": "value"
  },
  "payload": {                      // Optional: update payload
    "updated": true
  },
  "add_event_data": false           // Optional: update add_event_data flag
}
```

#### Delete Webhook

```http
DELETE /webhooks/{id}
Headers:
  KV-Namespace: myns
  KV-App-Name: myapp
```

#### Webhook Payload

When a webhook is triggered, the payload structure depends on the `add_event_data` setting:

**If `add_event_data` is `false` (default):**
Only your custom payload fields (if provided) are sent:

```json
{
  "source": "kv-store"  // Only custom payload fields
}
```

**If `add_event_data` is `true`:**
Event data is nested under an "event" key along with custom payload fields:

```json
{
  "source": "kv-store",  // Custom payload fields
  "event": {
    "event": "create",           // Event type: create, update, or delete
    "namespace": "myns",         // Namespace
    "appName": "myapp",          // App name
    "key": "foo",                // Key (without prefix)
    "value": "bar",              // Value (null for delete events)
    "ttl": 60,                   // TTL in seconds (if applicable)
    "expire_at": 1710000000,     // Expiration timestamp (if TTL set)
    "timestamp": 1710000000      // Unix timestamp
  }
}
```

**Empty payload:**
If no custom payload is provided and `add_event_data` is `false`, no payload data is sent.

**Webhook Headers:**
All webhook requests include the following headers:
- `Content-Type: application/json`
- `User-Agent: github.com/mrofi/simple-golang-kv`
- Any custom headers specified in the webhook registration

#### Webhook Events

- **create**: Triggered when a new key is created
- **update**: Triggered when an existing key is updated
- **delete**: Triggered when a key is deleted

#### Key Patterns

- **Exact match**: `"key": "foo"` - Matches only the exact key "foo"
- **Prefix match**: `"key": "foo*"` - Matches all keys starting with "foo" (e.g., "foo", "foobar", "foo123")

#### Watcher

The system includes a background watcher that monitors all key-value changes and automatically triggers matching webhooks. Only one pod runs the watcher at a time (enforced by distributed lock). If the watcher pod crashes, the lock expires (TTL 10s) and another pod automatically takes over, ensuring high availability.

## Development

- Go 1.25+
- etcd 3.x
- See `.github/workflows/ci.yml` for CI

## License

MIT