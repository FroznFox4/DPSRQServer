# DPSRQServer

gRPC-микросервис для синхронизации урона группы в реальном времени.

**GitHub:** `github.com/FroznFox4/DPSRQServer`
**Клиент:** RQWatcher (Rust/Tauri) — синхронизирует урон группы через gRPC.

---

## Архитектура

```
cmd/server/main.go              ← точка входа, graceful shutdown
cmd/party_scenario/main.go      ← интеграционный сценарий: 3 игрока в комнате
cmd/throttle_scenario/main.go   ← интеграционный сценарий: проверка throttle
internal/
└── partysync/
    ├── hub.go                  ← Hub: комнаты, снапшоты, throttled broadcast (300ms)
    ├── service.go              ← gRPC PartySync сервис
    ├── hub_test.go             ← unit-тесты Hub
    └── pb/                     ← сгенерированный protobuf-код (не .gitignore)
proto/
└── partysync.proto             ← source of truth gRPC контракта
```

**Стек:** Go 1.25, gRPC v1.68, protobuf v1.36.

---

## Сборка и запуск

```bash
make build   # → bin/server
make run     # go run ./cmd/server
```

**Переменные окружения:**

| Переменная | По умолчанию | Обязательна |
|-----------|-------------|-------------|
| `GRPC_ADDR` | `:50051` | Нет |

---

## gRPC (порт 50051)

**Сервис:** `PartySync::SyncDamage` — bidirectional stream.
**Auth:** отсутствует. Username берётся из поля `Player` первого сообщения.

```protobuf
rpc SyncDamage(stream DamageUpdate) returns (stream PartyState);
```

**Жизненный цикл стрима:**
1. `partySyncService.SyncDamage` ждёт первый `DamageUpdate` с непустыми `room` и `player`
2. `hub.Join(room, username, stream)` — регистрация
3. Loop: `stream.Recv()` → `hub.Update()` — сохраняет снапшот, помечает комнату dirty
4. Per-room тикер 300 мс — делает broadcast если dirty, сбрасывает флаг
5. Disconnect / EOF → `hub.Leave()`, комната удаляется если пустая

**Throttle:** каждая комната имеет одну горутину-тикер (`DefaultBroadcastInterval = 300ms`).
`Update()` только записывает снапшот и ставит `dirty=true`. Broadcast происходит не чаще раза в 300 мс,
все апдейты внутри одного тика батчатся в одну отправку `PartyState`.

**Регенерация pb-кода:**
```bash
make proto   # требует protoc + protoc-gen-go + protoc-gen-go-grpc в PATH
```
pb-файлы коммитятся в репозиторий (не игнорируются).

---

## Makefile

| Цель | Описание |
|------|----------|
| `proto` | Regenerate `internal/partysync/pb/` из `proto/partysync.proto` |
| `build` | Сборка в `bin/server` |
| `run` | `go run ./cmd/server` |
| `test` | `go test ./...` |

---

## Тесты

### Unit-тесты

```bash
make test          # все тесты
go test ./... -v   # с именами
```

**Hub-тесты** (`internal/partysync/hub_test.go`):

| Что тестируется |
|----------------|
| Join/Leave, broadcast, throttle (10 updates → 1 broadcast) |
| Изоляция комнат, cleanup, targets |

Используют `newHubWithInterval(5ms)` и `tick()` (`sleep 20ms`) для ожидания тика.

### Интеграционные сценарии

Требуют запущенного сервера (`make run`).

#### party_scenario — 3 игрока в одной комнате

```bash
go run ./cmd/party_scenario/
```

Что проверяет:
1. Каждый из 3 игроков открывает gRPC bidirectional stream
2. Каждый отправляет `DamageUpdate` со своим уроном и списком целей
3. Каждый ждёт `PartyState` пока не увидит всех 3 игроков
4. Проверяет: все 3 игрока видят консолидированный `PartyState` со всеми участниками

Ожидаемый вывод:
```
━━━ RESULT: ALL CHECKS PASSED ✓ ━━━
```

#### throttle_scenario — проверка батчинга broadcast

```bash
go run ./cmd/throttle_scenario/
```

Что проверяет:
1. Отправляет 10 `DamageUpdate` подряд (за ~700 мкс)
2. Слушает ответы 700 мс (покрывает ~2 тика по 300 мс)
3. Проверяет: получен **≤2 broadcast**
4. Проверяет: последний `TotalDamage` = значение из последнего апдейта

---

## История изменений

### Рефакторинг в чистый DPS-микросервис (14.03.2026)
- Удалены: HTTP API, авторизация, PostgreSQL, game_logs, leaderboard, stats
- Удалены зависимости: chi, pgx, bcrypt, golang-jwt
- Авторизация убрана: username берётся из поля `Player` первого gRPC-сообщения
- Единственная переменная окружения: `GRPC_ADDR` (по умолчанию `:50051`)

### feature/party-damage-sync (13.03.2026)
- `proto/partysync.proto` + `internal/partysync/` — gRPC PartySync bidirectional stream
- Hub throttle: `DefaultBroadcastInterval = 300ms`, per-room goroutine + `atomic.Bool dirty`
- `cmd/party_scenario/` — интеграционный сценарий: 3 игрока, консолидированный PartyState
- `cmd/throttle_scenario/` — интеграционный сценарий: 10 апдейтов → 1 broadcast

### Initial commit (11.02.2026)
- HTTP API: register, login, sync logs, stats, leaderboard
- Batch insert с дедупликацией через COPY + ON CONFLICT
- Материализованное view для leaderboard с фоновым refresh каждые 60с
