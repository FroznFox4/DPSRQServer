CREATE MATERIALIZED VIEW leaderboard_summary AS
SELECT
    u.id AS user_id,
    u.username,
    COALESCE(SUM(CASE WHEN gl.kind = 'gold' THEN gl.amount ELSE 0 END), 0) AS total_gold,
    COALESCE(SUM(CASE WHEN gl.kind = 'monster' THEN gl.exp ELSE 0 END), 0) AS total_exp,
    COALESCE(COUNT(CASE WHEN gl.kind = 'monster' THEN 1 END), 0) AS total_kills,
    COALESCE(COUNT(CASE WHEN gl.kind = 'item' THEN 1 END), 0) AS total_items
FROM users u
LEFT JOIN game_logs gl ON gl.user_id = u.id
GROUP BY u.id, u.username;

CREATE UNIQUE INDEX idx_lb_user ON leaderboard_summary(user_id);
