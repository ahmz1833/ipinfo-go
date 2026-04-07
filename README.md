# ipinfo-go

A fast, lightweight Go service for IP address information lookup with automatic database updates. Queries IP addresses against MaxMind ASN database and returns enriched metadata including CIDR blocks, ASN, organization name, country code, and domain information.

## Features

- **Multi-stream concurrent downloads**: Downloads IP database with configurable parallel connections (default: 12) with live progress logging
- **Multiple response formats**: HTML, JSON, and plain text output
- **Automatic database updates**: Periodic background updates every 24 hours
- **CIDR information**: Returns CIDR blocks for queried IPs
- **Web UI**: Responsive Tailwind CSS interface showing all IP details
- **Health checks**: Built-in database integrity validation on startup and load
- **Docker ready**: Containerized with multi-stage builds
- **Zero external dependencies** for downloading: Native Go HTTP implementation

## API Endpoints

### Query by IP Address

#### HTML Response (default for browsers)
```bash
curl -H "Accept: text/html" http://localhost:8080/1.1.1.1
```

#### JSON Response
```bash
curl http://localhost:8080/1.1.1.1?format=json
# or
curl -H "Accept: application/json" http://localhost:8080/1.1.1.1
```

#### Plain Text Response
```bash
curl http://localhost:8080/1.1.1.1?format=text
```

### Query Your Own IP
```bash
curl http://localhost:8080/
# Returns information about your client IP
```

### Query via Query Parameter
```bash
curl http://localhost:8080/?ip=8.8.8.8&format=json
```

## Response Format

### JSON
```json
{
  "ip": "1.1.1.1",
  "cidr": "1.1.1.0/24",
  "asn": 13335,
  "name": "CLOUDFLARENET",
  "org": "Cloudflare Inc.",
  "country_code": "US",
  "domain": "cloudflare.com"
}
```

### Plain Text
```
IP: 1.1.1.1
CIDR: 1.1.1.0/24
ASN: AS13335
Name: CLOUDFLARENET
Org: Cloudflare Inc.
Country: US
Domain: cloudflare.com
```

## Requirements

- Go 1.21+ (for building from source)
- For Docker: Docker and Docker Compose
- Network access to download IP database from GitHub

## Installation

### From Source

```bash
git clone https://github.com/ahmz1833/ipinfo-go
cd ipinfo-go
go mod download
go build .
./ipinfo
```

### Docker

```bash
docker build -t ipinfo-go .
docker run -p 8080:8080 -v ipinfo-cache:/var/cache/ipinfo ipinfo-go
```

### Docker Compose

```bash
docker-compose up -d
```

## Configuration

Configuration is currently built-in via constants in `config.go`:

| Setting | Default | Description |
|---------|---------|-------------|
| `dbURL` | GitHub raw URL | IP-to-ASN database source |
| `dbDir` | `/var/cache/ipinfo` | Local database cache directory |
| `updateFreq` | 24h | Automatic database update interval |
| `listenAddr` | `:8080` | HTTP server listen address |
| `downloadConnections` | 12 | Concurrent download streams |
| `progressInterval` | 1s | Download progress log interval |

## Project Structure

```
.
├── main.go           # Application bootstrap and server startup
├── config.go         # Configuration, constants, and data models
├── db.go             # Database lifecycle and IP lookup logic
├── downloader.go     # Multi-stream download with progress logging
├── handler.go        # HTTP request handling and response formatting
├── Dockerfile        # Container image definition
├── go.mod            # Go module dependencies
├── templates/
│   └── index.html    # Web UI template (Tailwind CSS)
└── .gitignore        # Git ignore rules
```

## Logging

The service logs all operations to stdout:
- Application startup and initialization
- Database cache validation
- Download progress (percentage, speed, total size)
- HTTP requests and responses
- Database integrity checks
- Background update operations

Example logs during startup:
```
2026/04/07 15:39:07 Initializing application...
2026/04/07 15:39:07 Templates parsed successfully
2026/04/07 15:39:07 Starting ipinfo service...
2026/04/07 15:39:07 Starting multi-stream download with 12 connections (size: 73.00 MB)
2026/04/07 15:39:10 Download progress: 45.3% (33.04 MB/73.00 MB) speed 11.23 MB/s
```

## Database

- **Source**: [IPLocate IP-to-ASN Database](https://github.com/iplocate/ip-address-databases)
- **Format**: MaxMind MMDB
- **Size**: ~73 MB
- **Update Frequency**: Every 24 hours (configurable)
- **Cache Location**: `/var/cache/ipinfo/ip-to-asn.mmdb`

The database is automatically:
1. Downloaded on first startup (blocking)
2. Background updated every 24 hours
3. Validated for integrity on load
4. Re-downloaded if validation fails

## Performance

- **Startup**: ~50 seconds (includes initial database download)
- **Requests**: <5ms typical response time
- **Database Size**: 76 MB (resident in memory)
- **Download**: Multi-stream parallel download with automatic fallback to single stream

## Development

### Build & Test
```bash
go build .
./ipinfo &
curl http://localhost:8080/8.8.8.8
```

### Format Code
```bash
gofmt -w *.go
```

### Run Tests
```bash
go test ./...
```

## License

MIT

## Support

For issues and feature requests, please open an issue on the repository.
