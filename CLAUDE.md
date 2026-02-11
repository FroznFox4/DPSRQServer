# Game Log Server

## Архитектура

Go HTTP-сервер, принимающий синхронизированные игровые логи от клиентов (Game Log Monitor) и предоставляющий лидерборды и статистику.

```
GameLogServer
├── HTTP API (chi router)
├── JWT Auth (bcrypt + HS256)
├── PostgreSQL (pgx pool)
├── Batch Sync (COPY protocol)
└── Leaderboard Cache (background goroutine)
```

---

## Сборка и запуск

### Требования
- Go 1.22+
- PostgreSQL 16 (или Docker)
- golang-migrate CLI

### Команды

```bash
# PostgreSQL
docker-compose up -d

# Миграции
make migrate-up

# Запуск сервера
JWT_SECRET=my-secret make run

# Сборка бинарника
make build          # → bin/server

# Тесты
make test
```

---

## Структура проекта

```
cmd/
└── server/
    └── main.go                  # Точка входа, wiring, graceful shutdown
internal/
├── config/
│   └── config.go                # Env-based: PORT, DATABASE_URL, JWT_SECRET
├── model/
│   └── models.go                # User, GameLog, SyncRequest/Response, LeaderboardEntry, UserStats
├── handler/
│   ├── auth.go                  # POST /register, POST /login + writeJSON/writeError хелперы
│   ├── sync.go                  # POST /logs/sync
│   ├── leaderboard.go           # GET /leaderboard/{category}
│   └── stats.go                 # GET /stats/me
├── middleware/
│   └── auth.go                  # JWT validation, context keys (UserIDKey, UsernameKey)
├── repository/
│   ├── user.go                  # Create, GetByUsername
│   ├── gamelog.go               # BatchInsert (temp table + CopyFrom + ON CONFLICT), GetUserStats
│   └── leaderboard.go           # GetLeaderboard, RefreshView
└── service/
    ├── auth.go                  # Register, Login, generateToken, ValidateToken
    ├── sync.go                  # SyncLogs (валидация batch size, делегация в repo)
    ├── leaderboard.go           # GetLeaderboard (cache), StartRefresh (background goroutine)
    └── stats.go                 # GetUserStats (делегация в repo)
migrations/
├── 001_create_users.up.sql
├── 001_create_users.down.sql
├── 002_create_game_logs.up.sql
├── 002_create_game_logs.down.sql
├── 003_create_leaderboard_view.up.sql
└── 003_create_leaderboard_view.down.sql
```

---

## Конфигурация

Файл: `internal/config/config.go`

| Переменная | Значение по умолчанию | Описание |
|------------|----------------------|----------|
| `PORT` | `8080` | Порт сервера |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/gamelogserver?sslmode=disable` | PostgreSQL connection string |
| `JWT_SECRET` | — (обязательно) | Секрет для подписи JWT |

---

## API Endpoints

Все эндпоинты под `/api/v1`.

### Публичные

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/auth/register` | Регистрация, возвращает JWT |
| POST | `/auth/login` | Логин, возвращает JWT |
| GET | `/leaderboard/{category}` | Лидерборд (exp, gold, kills, items) |

### Защищённые (требуют `Authorization: Bearer <token>`)

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/logs/sync` | Загрузка логов (batch до 1000) |
| GET | `/stats/me` | Статистика текущего пользователя |

---

## PostgreSQL Schema

### users
```sql
id SERIAL PRIMARY KEY
username VARCHAR(50) UNIQUE NOT NULL
password_hash VARCHAR(255) NOT NULL
created_at TIMESTAMPTZ DEFAULT NOW()
```

### game_logs
```sql
id BIGSERIAL PRIMARY KEY
user_id INTEGER NOT NULL REFERENCES users(id)
client_id INTEGER NOT NULL              -- SQLite id клиента (для дедупликации)
timestamp TIMESTAMPTZ NOT NULL
time TEXT
kind VARCHAR(20) NOT NULL               -- gold, item, monster, other
action VARCHAR(20)
name TEXT
amount BIGINT
count INTEGER
exp BIGINT
raw_text TEXT
synced_at TIMESTAMPTZ DEFAULT NOW()
UNIQUE(user_id, client_id)              -- дедупликация по клиенту
```

Индексы: `user_id`, `kind`, `timestamp`, `(user_id, kind)`.

### leaderboard_summary (MATERIALIZED VIEW)
```sql
user_id, username, total_gold, total_exp, total_kills, total_items
```
Обновляется `REFRESH MATERIALIZED VIEW CONCURRENTLY` каждые 60 секунд фоновой горутиной.

---

## Ключевые паттерны

### Batch Insert (sync endpoint)
1. Создание `TEMP TABLE _staging ON COMMIT DROP`
2. `pgx.CopyFrom` (COPY protocol) в temp таблицу
3. `INSERT INTO game_logs ... SELECT FROM _staging ON CONFLICT DO NOTHING`
4. Всё в одной транзакции

### Leaderboard Cache
- `sync.RWMutex` защищает `map[string][]LeaderboardEntry`
- Фоновая горутина: `REFRESH MATERIALIZED VIEW` + query → cache каждые 60с
- При cache miss — прямой запрос в БД

### JWT Auth
- HS256, 72 часа expiration
- Middleware извлекает `user_id` и `username` в `context.Context`
- Хелпер `middleware.GetUserID(ctx)` для хэндлеров

---

## Слои приложения

```
Handler → Service → Repository → pgxpool.Pool → PostgreSQL
```

- **Handler**: HTTP decode/encode, вызов сервиса, возврат JSON
- **Service**: бизнес-логика, валидация, кэширование
- **Repository**: SQL-запросы через pgx

Все зависимости инжектятся через конструкторы в `main.go`.

---

## Тестирование

```bash
make test
```

---

## Docker

```bash
# PostgreSQL для разработки
docker-compose up -d

# Сборка образа сервера
docker build -t gamelogserver .
```

Multi-stage build: `golang:1.22-alpine` → `alpine:3.19`.
