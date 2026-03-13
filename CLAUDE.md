# DPSRQServer

Go-бэкенд для хранения игровых логов, таблиц лидеров и синхронизации урона группы в реальном времени.

**GitHub:** `github.com/FroznFox4/DPSRQServer`
**Клиент:** RQWatcher (Rust/Tauri) — отправляет логи через HTTP и синхронизирует урон группы через gRPC.

---

## Архитектура

```
cmd/server/main.go          ← точка входа, wiring, graceful shutdown
internal/
├── config/config.go        ← конфигурация из env
├── model/models.go         ← доменные типы (User, GameLog, etc.)
├── handler/                ← HTTP-обработчики (chi)
│   ├── auth.go             ← POST /register, /login
│   ├── sync.go             ← POST /logs/sync
│   ├── leaderboard.go      ← GET /leaderboard/{category}
│   └── stats.go            ← GET /stats/me
├── middleware/auth.go       ← JWT middleware, GetUserID(ctx)
├── service/                ← бизнес-логика
│   ├── auth.go             ← Register, Login, ValidateToken
│   ├── sync.go             ← SyncLogs (batch validation)
│   ├── leaderboard.go      ← GetLeaderboard + cache + background refresh
│   └── stats.go            ← GetUserStats (делегация)
├── repository/             ← доступ к PostgreSQL (pgx)
│   ├── user.go             ← Create, GetByUsername
│   ├── gamelog.go          ← BatchInsert (COPY + ON CONFLICT), GetUserStats
│   └── leaderboard.go      ← GetLeaderboard, RefreshView
└── partysync/              ← gRPC bidirectional sync
    ├── hub.go              ← Hub: комнаты, снапшоты, broadcast
    ├── service.go          ← gRPC PartySync сервис с JWT auth
    └── pb/                 ← сгенерированный protobuf-код (не .gitignore)
proto/
└── partysync.proto         ← source of truth gRPC контракта
migrations/                 ← SQL (golang-migrate)
```

**Стек:** Go 1.25, chi v5, pgx v5, golang-jwt v5, bcrypt, gRPC v1.68, protobuf v1.36, PostgreSQL 16.

---

## Сборка и запуск

```bash
# Зависимости
docker-compose up -d    # PostgreSQL
make migrate-up         # применить миграции

# Запуск
JWT_SECRET=my-secret make run   # dev
make build                      # → bin/server
```

**Переменные окружения:**

| Переменная | По умолчанию | Обязательна |
|-----------|-------------|-------------|
| `JWT_SECRET` | — | **Да** |
| `PORT` | `8080` | Нет |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/gamelogserver?sslmode=disable` | Нет |

---

## HTTP API

**Base:** `/api/v1`

### Публичные

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/auth/register` | Регистрация (username 3–50 симв, password ≥6) |
| POST | `/auth/login` | Логин → JWT (72ч, HS256) |
| GET | `/leaderboard/{category}?limit=10` | Лидерборд (`exp`/`gold`/`kills`/`items`, limit 1–100) |

### Защищённые (`Authorization: Bearer <jwt>`)

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/logs/sync` | Batch-загрузка логов (макс 1000 за раз) |
| GET | `/stats/me` | Статистика текущего пользователя |

Ошибки: `{"error": "описание"}`.

---

## gRPC (порт 50051)

**Сервис:** `PartySync::SyncDamage` — bidirectional stream.
**Auth:** JWT в metadata `authorization: Bearer <token>`.

```protobuf
rpc SyncDamage(stream DamageUpdate) returns (stream PartyState);
```

**Жизненный цикл стрима:**
1. `authStreamInterceptor` валидирует JWT из metadata
2. `partySyncService.SyncDamage` ждёт первый `DamageUpdate` с `room`
3. `hub.Join(room, username, stream)` — регистрация
4. Loop: `stream.Recv()` → `hub.Update()` → broadcast `PartyState` всем в комнате
5. Disconnect / EOF → `hub.Leave()`, комната удаляется если пустая

**Регенерация pb-кода:**
```bash
make proto   # требует protoc + protoc-gen-go + protoc-gen-go-grpc в PATH
```
pb-файлы коммитятся в репозиторий (не игнорируются).

---

## База данных

### Схема

**`users`:** `id, username UNIQUE, password_hash, created_at`

**`game_logs`:**
```
id, user_id, client_id, timestamp, time, kind, action, name, amount, count, exp, raw_text, synced_at
UNIQUE(user_id, client_id)  — дедупликация
```
`kind`: `gold`, `monster`, `item`, `damage`, `other`

**`leaderboard_summary`** — материализованное view, refresh каждые 60 сек:
```sql
(user_id, username, total_gold, total_exp, total_kills, total_items)
```

### BatchInsert

`GameLogRepo.BatchInsert`: BEGIN → `CREATE TEMP TABLE _staging ON COMMIT DROP` → `COPY` → `INSERT ... SELECT ... ON CONFLICT DO NOTHING` → COMMIT.

---

## Интерфейсы для тестируемости

Каждый сервис принимает **интерфейс** репозитория (определён в service-пакете):

```go
// service/auth.go
type userRepository interface {
    Create(ctx, username, hash) (*model.User, error)
    GetByUsername(ctx, username) (*model.User, error)
}

// service/sync.go
type gameLogRepository interface {
    BatchInsert(ctx, userID, logs) (inserted, duplicates int, error)
}

// service/stats.go
type statsLogRepository interface {
    GetUserStats(ctx, userID) (*model.UserStats, error)
}
```

Каждый handler принимает **интерфейс** сервиса (определён в handler-пакете):
```go
// handler/auth.go   → authServiceIface
// handler/sync.go   → syncServiceIface
// handler/leaderboard.go → leaderboardServiceIface
// handler/stats.go  → statsServiceIface
```

`*repository.*Repo` и `*service.*Service` удовлетворяют интерфейсам неявно → `main.go` не меняется.

---

## Тесты

```bash
make test          # все тесты
go test ./... -v   # с именами
go test -run Hub   # конкретный паттерн
```

**44 unit-теста** — все без реальной БД:

| Пакет | Что тестируется |
|-------|----------------|
| `internal/partysync` | Hub: Join/Leave, broadcast, изоляция комнат, cleanup, targets |
| `internal/service` | AuthService: Register + все ошибки, Login, ValidateToken |
| `internal/service` | SyncService: empty batch, maxBatchSize, ошибки репо, duplicates |
| `internal/middleware` | JWT: header, Bearer scheme, tampered, wrong secret, expired |
| `internal/handler` | Auth/Sync/Leaderboard/Stats: happy path + 401/400/500 |

**Моки:** определены в `_test.go` файлах (не в production-коде). Имена: `mockUserRepo`, `mockGameLogRepo`, `mockAuthService`, `mockSyncService`, `mockLeaderboardService`, `mockStatsService`.

**Не покрыто:** repository-слой (требует реальной БД), `cmd/server/main.go`.

---

## JWT

Алгоритм: **HS256**. Claims:
```go
jwt.MapClaims{
    "user_id":  userID,   // float64 при декодировании → int()
    "username": username,
    "exp":      now + 72h,
    "iat":      now,
}
```
`ValidateToken` возвращает `(userID int, username string, error)`.

---

## Makefile

| Цель | Описание |
|------|----------|
| `proto` | Regenerate `internal/partysync/pb/` из `proto/partysync.proto` |
| `build` | Сборка в `bin/server` |
| `run` | `go run ./cmd/server` |
| `test` | `go test ./...` |
| `migrate-up/down` | Управление миграциями |
| `docker-up/down` | PostgreSQL контейнер |

---

## История изменений

### feature/party-damage-sync (13.03.2026)
- `proto/partysync.proto` + `internal/partysync/` — gRPC PartySync bidirectional stream
- `cmd/server/main.go` — gRPC сервер на `:50051` + `authStreamInterceptor`
- `go.mod` — добавлены `grpc v1.68.0`, `protobuf v1.36.0`
- Интерфейсы в service и handler пакетах для тестируемости
- **44 unit-теста** для hub, auth service, sync service, middleware, handlers
- Репо переименовано: `GameLogServer` → `DPSRQServer`

### Initial commit (11.02.2026)
- HTTP API: register, login, sync logs, stats, leaderboard
- Batch insert с дедупликацией через COPY + ON CONFLICT
- Материализованное view для leaderboard с фоновым refresh каждые 60с
- Docker + golang-migrate
