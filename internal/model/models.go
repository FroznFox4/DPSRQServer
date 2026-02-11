package model

import "time"

type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type GameLog struct {
	ID        int64     `json:"id"`
	UserID    int       `json:"user_id"`
	ClientID  int       `json:"client_id"`
	Timestamp time.Time `json:"timestamp"`
	Time      *string   `json:"time"`
	Kind      string    `json:"kind"`
	Action    *string   `json:"action"`
	Name      *string   `json:"name"`
	Amount    *int64    `json:"amount"`
	Count     *int32    `json:"count"`
	Exp       *int64    `json:"exp"`
	RawText   *string   `json:"raw_text"`
	SyncedAt  time.Time `json:"synced_at"`
}

type SyncRequest struct {
	Logs []SyncLogEntry `json:"logs"`
}

type SyncLogEntry struct {
	ClientID  int     `json:"client_id"`
	Timestamp string  `json:"timestamp"`
	Time      *string `json:"time"`
	Kind      string  `json:"kind"`
	Action    *string `json:"action"`
	Name      *string `json:"name"`
	Amount    *int64  `json:"amount"`
	Count     *int32  `json:"count"`
	Exp       *int64  `json:"exp"`
	RawText   *string `json:"raw_text"`
}

type SyncResponse struct {
	Synced     int `json:"synced"`
	Duplicates int `json:"duplicates"`
}

type LeaderboardEntry struct {
	Rank     int    `json:"rank"`
	Username string `json:"username"`
	Value    int64  `json:"value"`
}

type UserStats struct {
	TotalMonsters int             `json:"total_monsters"`
	TotalExp      int64           `json:"total_exp"`
	TotalGold     int64           `json:"total_gold"`
	TotalItems    int             `json:"total_items"`
	Monsters      []NameCountPair `json:"monsters"`
	Items         []NameCountPair `json:"items"`
}

type NameCountPair struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	ID       int    `json:"id,omitempty"`
	Username string `json:"username,omitempty"`
	Token    string `json:"token"`
}
