# simple-golang-kv

A simple key-value store API built with Go, Echo, and etcd.

## Features

- RESTful API for key-value operations
- Namespace and app name isolation via headers
- TTL (time-to-live) support for expiring keys
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

## Development

- Go 1.25+
- etcd 3.x
- See `.github/workflows/ci.yml` for CI

## License

MIT