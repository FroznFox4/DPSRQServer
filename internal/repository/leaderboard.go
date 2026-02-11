package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kirill/gamelogserver/internal/model"
)

type LeaderboardRepo struct {
	pool *pgxpool.Pool
}

func NewLeaderboardRepo(pool *pgxpool.Pool) *LeaderboardRepo {
	return &LeaderboardRepo{pool: pool}
}

var categoryColumns = map[string]string{
	"exp":   "total_exp",
	"gold":  "total_gold",
	"kills": "total_kills",
	"items": "total_items",
}

func (r *LeaderboardRepo) GetLeaderboard(ctx context.Context, category string, limit int) ([]model.LeaderboardEntry, error) {
	col, ok := categoryColumns[category]
	if !ok {
		return nil, fmt.Errorf("unknown category: %s", category)
	}

	query := fmt.Sprintf(`
		SELECT username, %s AS value
		FROM leaderboard_summary
		WHERE %s > 0
		ORDER BY %s DESC
		LIMIT $1
	`, col, col, col)

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.LeaderboardEntry
	rank := 1
	for rows.Next() {
		var e model.LeaderboardEntry
		if err := rows.Scan(&e.Username, &e.Value); err != nil {
			return nil, err
		}
		e.Rank = rank
		rank++
		entries = append(entries, e)
	}

	if entries == nil {
		entries = []model.LeaderboardEntry{}
	}
	return entries, nil
}

func (r *LeaderboardRepo) RefreshView(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, "REFRESH MATERIALIZED VIEW CONCURRENTLY leaderboard_summary")
	return err
}
