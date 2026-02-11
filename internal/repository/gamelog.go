package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kirill/gamelogserver/internal/model"
)

type GameLogRepo struct {
	pool *pgxpool.Pool
}

func NewGameLogRepo(pool *pgxpool.Pool) *GameLogRepo {
	return &GameLogRepo{pool: pool}
}

// BatchInsert uses a temp table + INSERT ... ON CONFLICT to handle dedup efficiently.
// Returns (inserted, duplicates).
func (r *GameLogRepo) BatchInsert(ctx context.Context, userID int, logs []model.SyncLogEntry) (int, int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	// Create temp table
	_, err = tx.Exec(ctx, `
		CREATE TEMP TABLE _staging (
			client_id INTEGER,
			timestamp TIMESTAMPTZ,
			time TEXT,
			kind VARCHAR(20),
			action VARCHAR(20),
			name TEXT,
			amount BIGINT,
			count INTEGER,
			exp BIGINT,
			raw_text TEXT
		) ON COMMIT DROP
	`)
	if err != nil {
		return 0, 0, err
	}

	// COPY into temp table
	rows := make([][]interface{}, len(logs))
	for i, l := range logs {
		ts, parseErr := time.Parse(time.RFC3339, l.Timestamp)
		if parseErr != nil {
			ts, parseErr = time.Parse("2006-01-02 15:04:05", l.Timestamp)
			if parseErr != nil {
				ts = time.Now()
			}
		}
		rows[i] = []interface{}{
			l.ClientID, ts, l.Time, l.Kind, l.Action,
			l.Name, l.Amount, l.Count, l.Exp, l.RawText,
		}
	}

	_, err = tx.CopyFrom(ctx, pgx.Identifier{"_staging"},
		[]string{"client_id", "timestamp", "time", "kind", "action", "name", "amount", "count", "exp", "raw_text"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return 0, 0, err
	}

	// Insert from staging with dedup
	tag, err := tx.Exec(ctx, `
		INSERT INTO game_logs (user_id, client_id, timestamp, time, kind, action, name, amount, count, exp, raw_text)
		SELECT $1, client_id, timestamp, time, kind, action, name, amount, count, exp, raw_text
		FROM _staging
		ON CONFLICT (user_id, client_id) DO NOTHING
	`, userID)
	if err != nil {
		return 0, 0, err
	}

	if err = tx.Commit(ctx); err != nil {
		return 0, 0, err
	}

	inserted := int(tag.RowsAffected())
	duplicates := len(logs) - inserted
	return inserted, duplicates, nil
}

func (r *GameLogRepo) GetUserStats(ctx context.Context, userID int) (*model.UserStats, error) {
	var stats model.UserStats

	err := r.pool.QueryRow(ctx, `
		SELECT
			COALESCE(COUNT(CASE WHEN kind = 'monster' THEN 1 END), 0),
			COALESCE(SUM(CASE WHEN kind = 'monster' THEN exp ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN kind = 'gold' THEN amount ELSE 0 END), 0),
			COALESCE(COUNT(CASE WHEN kind = 'item' THEN 1 END), 0)
		FROM game_logs WHERE user_id = $1
	`, userID).Scan(&stats.TotalMonsters, &stats.TotalExp, &stats.TotalGold, &stats.TotalItems)
	if err != nil {
		return nil, err
	}

	// Top monsters
	monsterRows, err := r.pool.Query(ctx, `
		SELECT name, COUNT(*) as cnt
		FROM game_logs
		WHERE user_id = $1 AND kind = 'monster' AND name IS NOT NULL
		GROUP BY name ORDER BY cnt DESC LIMIT 20
	`, userID)
	if err != nil {
		return nil, err
	}
	defer monsterRows.Close()

	for monsterRows.Next() {
		var p model.NameCountPair
		if err := monsterRows.Scan(&p.Name, &p.Count); err != nil {
			return nil, err
		}
		stats.Monsters = append(stats.Monsters, p)
	}

	// Top items
	itemRows, err := r.pool.Query(ctx, `
		SELECT name, COALESCE(SUM(count), COUNT(*)) as cnt
		FROM game_logs
		WHERE user_id = $1 AND kind = 'item' AND name IS NOT NULL
		GROUP BY name ORDER BY cnt DESC LIMIT 20
	`, userID)
	if err != nil {
		return nil, err
	}
	defer itemRows.Close()

	for itemRows.Next() {
		var p model.NameCountPair
		if err := itemRows.Scan(&p.Name, &p.Count); err != nil {
			return nil, err
		}
		stats.Items = append(stats.Items, p)
	}

	if stats.Monsters == nil {
		stats.Monsters = []model.NameCountPair{}
	}
	if stats.Items == nil {
		stats.Items = []model.NameCountPair{}
	}

	return &stats, nil
}
