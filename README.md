# Game Log Server

Cloud server for [Game Log Monitor](../GameLogMonitor). Receives synced game logs from multiple clients, provides leaderboards and cross-user statistics.

## Tech Stack

- **Go** with chi router
- **PostgreSQL 16** with pgx/v5 connection pool
- **JWT** authentication (bcrypt password hashing)
- **Docker Compose** for local development

## Quick Start

```bash
# 1. Start PostgreSQL
docker-compose up -d

# 2. Install migrate tool
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# 3. Run migrations
make migrate-up

# 4. Start server
JWT_SECRET=my-secret make run
```

Server starts on `http://localhost:8080`.

## Configuration

Environment variables (see `.env.example`):

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server port |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/gamelogserver?sslmode=disable` | PostgreSQL connection string |
| `JWT_SECRET` | — (required) | Secret key for JWT signing |

## API

All endpoints are prefixed with `/api/v1`.

### Auth (public)

```bash
# Register
curl -X POST localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"player1","password":"secret123"}'
# → {"id":1,"username":"player1","token":"eyJ..."}

# Login
curl -X POST localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"player1","password":"secret123"}'
# → {"token":"eyJ..."}
```

### Sync Logs (requires JWT)

```bash
curl -X POST localhost:8080/api/v1/logs/sync \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "logs": [
      {"client_id":1,"timestamp":"2025-01-21T12:00:00Z","kind":"monster","name":"Dragon","exp":500},
      {"client_id":2,"timestamp":"2025-01-21T12:01:00Z","kind":"gold","amount":150}
    ]
  }'
# → {"synced":2,"duplicates":0}
```

Max 1000 entries per batch. Duplicates are skipped by `(user_id, client_id)` unique constraint.

### Leaderboard (public)

```bash
curl localhost:8080/api/v1/leaderboard/exp?limit=10
# → [{"rank":1,"username":"player1","value":50000}, ...]
```

Categories: `exp`, `gold`, `kills`, `items`.

### User Stats (requires JWT)

```bash
curl localhost:8080/api/v1/stats/me \
  -H "Authorization: Bearer <token>"
# → {"total_monsters":120,"total_exp":50000,"total_gold":8000,"total_items":45,"monsters":[...],"items":[...]}
```

## Makefile Commands

| Command | Description |
|---------|-------------|
| `make build` | Build binary to `bin/server` |
| `make run` | Run server with `go run` |
| `make test` | Run all tests |
| `make migrate-up` | Apply all migrations |
| `make migrate-down` | Rollback all migrations |
| `make docker-up` | Start PostgreSQL container |
| `make docker-down` | Stop PostgreSQL container |

## Docker Build

```bash
docker build -t gamelogserver .
docker run -p 8080:8080 \
  -e DATABASE_URL=postgres://... \
  -e JWT_SECRET=my-secret \
  gamelogserver
```
