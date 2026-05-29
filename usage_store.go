package main

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type UsageStore struct {
	db  *sql.DB
	mu  sync.RWMutex
	dir string
}

func NewUsageStore(configDir string) (*UsageStore, error) {
	dbPath := configDir + "/usage.db"
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("打开用量数据库失败: %w", err)
	}

	store := &UsageStore{db: db, dir: configDir}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *UsageStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS usage_records (
			id              TEXT    PRIMARY KEY,
			provider        TEXT    NOT NULL DEFAULT '',
			profile_name    TEXT    NOT NULL DEFAULT '',
			model           TEXT    NOT NULL DEFAULT '',
			endpoint        TEXT    NOT NULL DEFAULT '',
			prompt_tokens   INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens    INTEGER NOT NULL DEFAULT 0,
			success         INTEGER NOT NULL DEFAULT 1,
			status_code     INTEGER NOT NULL DEFAULT 200,
			duration_ms     INTEGER NOT NULL DEFAULT 0,
			created_at      DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_usage_provider   ON usage_records(provider);
		CREATE INDEX IF NOT EXISTS idx_usage_created_at ON usage_records(created_at);
		PRAGMA user_version = 1;
	`)
	return err
}

func (s *UsageStore) Close() error {
	return s.db.Close()
}

func (s *UsageStore) Insert(record *UsageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT INTO usage_records
			(id, provider, profile_name, model, endpoint, prompt_tokens, completion_tokens, total_tokens, success, status_code, duration_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.Provider,
		record.ProfileName,
		record.Model,
		record.Endpoint,
		record.PromptTokens,
		record.CompletionTokens,
		record.TotalTokens,
		boolToInt(record.Success),
		record.StatusCode,
		record.DurationMs,
		record.CreatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *UsageStore) QueryStats() (UsageStatsResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	loc := now.Location()

	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	weekday := now.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	weekStart := todayStart.AddDate(0, 0, -int(weekday-time.Monday))
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, loc)
	thirtyDaysAgo := todayStart.AddDate(0, 0, -29)

	today, err := s.querySince(todayStart)
	if err != nil {
		return UsageStatsResponse{}, err
	}
	week, err := s.querySince(weekStart)
	if err != nil {
		return UsageStatsResponse{}, err
	}
	month, err := s.querySince(monthStart)
	if err != nil {
		return UsageStatsResponse{}, err
	}
	year, err := s.querySince(yearStart)
	if err != nil {
		return UsageStatsResponse{}, err
	}
	models, err := s.queryModelStats(monthStart)
	if err != nil {
		return UsageStatsResponse{}, err
	}
	ts, err := s.queryTimeSeries(thirtyDaysAgo)
	if err != nil {
		return UsageStatsResponse{}, err
	}
	return UsageStatsResponse{
		Today:      today,
		ThisWeek:   week,
		ThisMonth:  month,
		ThisYear:   year,
		Models:     models,
		TimeSeries: ts,
	}, nil
}

func (s *UsageStore) querySince(since time.Time) ([]UsageStats, error) {
	rows, err := s.db.Query(`
		SELECT
			COALESCE(provider, '') as provider,
			COUNT(*) as request_count,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as success_count,
			SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) as failure_count,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) as completion_tokens,
			COALESCE(CAST(AVG(duration_ms) AS REAL), 0) as avg_duration_ms
		FROM usage_records
		WHERE created_at >= ?
		GROUP BY provider
		ORDER BY request_count DESC
	`, since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []UsageStats
	for rows.Next() {
		var s UsageStats
		if err := rows.Scan(
			&s.Provider,
			&s.RequestCount,
			&s.SuccessCount,
			&s.FailureCount,
			&s.TotalTokens,
			&s.PromptTokens,
			&s.CompletionTokens,
			&s.AvgDurationMs,
		); err != nil {
			continue
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if stats == nil {
		stats = []UsageStats{}
	}
	return stats, nil
}

func (s *UsageStore) queryModelStats(since time.Time) ([]ModelStats, error) {
	rows, err := s.db.Query(`
		SELECT
			COALESCE(provider, '') as provider,
			COALESCE(model, '') as model,
			COUNT(*) as request_count,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as success_count,
			SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) as failure_count,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) as completion_tokens,
			COALESCE(CAST(AVG(duration_ms) AS REAL), 0) as avg_duration_ms
		FROM usage_records
		WHERE created_at >= ?
		GROUP BY provider, model
		ORDER BY total_tokens DESC
	`, since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []ModelStats
	for rows.Next() {
		var s ModelStats
		if err := rows.Scan(
			&s.Provider,
			&s.Model,
			&s.RequestCount,
			&s.SuccessCount,
			&s.FailureCount,
			&s.TotalTokens,
			&s.PromptTokens,
			&s.CompletionTokens,
			&s.AvgDurationMs,
		); err != nil {
			continue
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if stats == nil {
		stats = []ModelStats{}
	}
	return stats, nil
}

func (s *UsageStore) queryTimeSeries(since time.Time) ([]TimeSeriesPoint, error) {
	rows, err := s.db.Query(`
		SELECT
			date(created_at) as date,
			COUNT(*) as request_count,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) as completion_tokens
		FROM usage_records
		WHERE created_at >= ?
		GROUP BY date(created_at)
		ORDER BY date ASC
	`, since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []TimeSeriesPoint
	for rows.Next() {
		var p TimeSeriesPoint
		if err := rows.Scan(
			&p.Date,
			&p.RequestCount,
			&p.TotalTokens,
			&p.PromptTokens,
			&p.CompletionTokens,
		); err != nil {
			continue
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if points == nil {
		points = []TimeSeriesPoint{}
	}
	return points, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
