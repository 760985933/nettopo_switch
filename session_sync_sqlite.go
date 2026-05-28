package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// dbFileBasename is the Codex SQLite state file name.
const dbFileBasename = "state_5.sqlite"

func stateDBPath(codexHome string) string {
	return filepath.Join(codexHome, dbFileBasename)
}

// isSQLiteBusyError checks for database-locked errors.
func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "busy")
}

// isSQLiteMalformedError checks for corrupt database errors.
func isSQLiteMalformedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database disk image is malformed") ||
		strings.Contains(msg, "sqlite_corrupt") ||
		strings.Contains(msg, "malformed") ||
		strings.Contains(msg, "not a database")
}

// ---------- diagnostic reads ----------

// readSQLiteProviderCounts returns provider distribution in the threads table.
func readSQLiteProviderCounts(codexHome string) (*SyncRolloutInfo, bool, error) {
	dbPath := stateDBPath(codexHome)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, false, nil
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, false, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT
			CASE
				WHEN model_provider IS NULL OR model_provider = '' THEN '(missing)'
				ELSE model_provider
			END AS model_provider,
			archived,
			COUNT(*) AS count
		FROM threads
		GROUP BY model_provider, archived
		ORDER BY archived, model_provider
	`)
	if err != nil {
		if isSQLiteMalformedError(err) {
			return nil, true, err
		}
		return nil, false, err
	}
	defer rows.Close()

	result := &SyncRolloutInfo{
		Sessions:         ProviderCounts{},
		ArchivedSessions: ProviderCounts{},
	}
	for rows.Next() {
		var provider string
		var archived bool
		var count int
		if err := rows.Scan(&provider, &archived, &count); err != nil {
			continue
		}
		if archived {
			result.ArchivedSessions[provider] = count
		} else {
			result.Sessions[provider] = count
		}
	}
	return result, false, rows.Err()
}

// tableHasColumn checks if a column exists in the given table.
func tableHasColumn(db *sql.DB, tableName, columnName string) bool {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk bool
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			continue
		}
		if name == columnName {
			return true
		}
	}
	return false
}

// readSQLiteRepairStats counts rows needing has_user_event or cwd repair.
func readSQLiteRepairStats(codexHome string, userEventThreadIDs map[string]bool, threadCwdByID map[string]string) (*SyncRepairStats, error) {
	dbPath := stateDBPath(codexHome)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, nil
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	stats := &SyncRepairStats{}

	if tableHasColumn(db, "threads", "has_user_event") && len(userEventThreadIDs) > 0 {
		stmt, err := db.Prepare("SELECT has_user_event FROM threads WHERE id = ?")
		if err == nil {
			defer stmt.Close()
			for id := range userEventThreadIDs {
				var hasUE sql.NullInt64
				if err := stmt.QueryRow(id).Scan(&hasUE); err != nil {
					continue
				}
				if !hasUE.Valid || hasUE.Int64 != 1 {
					stats.UserEventRowsNeedingRepair++
				}
			}
		}
	}

	if tableHasColumn(db, "threads", "cwd") && len(threadCwdByID) > 0 {
		stmt, err := db.Prepare("SELECT cwd FROM threads WHERE id = ?")
		if err == nil {
			defer stmt.Close()
			for id, cwd := range threadCwdByID {
				if id == "" || strings.TrimSpace(cwd) == "" {
					continue
				}
				var dbCwd sql.NullString
				if err := stmt.QueryRow(id).Scan(&dbCwd); err != nil {
					continue
				}
				if dbCwd.Valid && dbCwd.String != cwd {
					stats.CwdRowsNeedingRepair++
				}
			}
		}
	}

	return stats, nil
}

// assertSQLiteWritable checks if the database can be locked for writing.
func assertSQLiteWritable(codexHome string) (bool, error) {
	dbPath := stateDBPath(codexHome)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return false, nil
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return false, err
	}
	defer db.Close()

	_, err = db.Exec("BEGIN IMMEDIATE")
	if err != nil {
		if isSQLiteBusyError(err) {
			return true, fmt.Errorf("state_5.sqlite is currently in use — close Codex and retry")
		}
		return true, err
	}
	db.Exec("ROLLBACK")
	return true, nil
}

// ---------- sync operations ----------

type sqliteSyncOptions struct {
	UserEventThreadIDs map[string]bool
	ThreadCwdByID      map[string]string
}

type sqliteSyncResult struct {
	DatabasePresent           bool
	ProviderRowsUpdated       int
	UserEventRowsUpdated      int
	CwdRowsUpdated            int
	TotalUpdated              int
}

// syncSQLite updates model_provider, has_user_event, and cwd in the Codex SQLite database.
func syncSQLite(codexHome string, targetProvider string, opts sqliteSyncOptions, afterUpdate func(result sqliteSyncResult) error) (sqliteSyncResult, error) {
	dbPath := stateDBPath(codexHome)
	result := sqliteSyncResult{}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		if afterUpdate != nil {
			_ = afterUpdate(result)
		}
		return result, nil
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return result, err
	}
	defer db.Close()

	result.DatabasePresent = true

	tx, err := db.Begin()
	if err != nil {
		if isSQLiteBusyError(err) {
			return result, fmt.Errorf("state_5.sqlite is currently in use — close Codex and retry")
		}
		if isSQLiteMalformedError(err) {
			return result, fmt.Errorf("state_5.sqlite is malformed — repair or restore from backup")
		}
		return result, err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	// 1. Update model_provider
	res, err := tx.Exec(
		"UPDATE threads SET model_provider = ? WHERE COALESCE(model_provider, '') <> ?",
		targetProvider, targetProvider,
	)
	if err != nil {
		return result, err
	}
	affected, _ := res.RowsAffected()
	result.ProviderRowsUpdated = int(affected)

	// 2. Repair has_user_event
	if tableHasColumn(db, "threads", "has_user_event") && len(opts.UserEventThreadIDs) > 0 {
		stmt, err := tx.Prepare(
			"UPDATE threads SET has_user_event = 1 WHERE id = ? AND COALESCE(has_user_event, 0) <> 1",
		)
		if err == nil {
			for id := range opts.UserEventThreadIDs {
				res, err := stmt.Exec(id)
				if err == nil {
					c, _ := res.RowsAffected()
					result.UserEventRowsUpdated += int(c)
				}
			}
			stmt.Close()
		}
	}

	// 3. Repair cwd paths
	if tableHasColumn(db, "threads", "cwd") && len(opts.ThreadCwdByID) > 0 {
		stmt, err := tx.Prepare(
			"UPDATE threads SET cwd = ? WHERE id = ? AND COALESCE(cwd, '') <> ?",
		)
		if err == nil {
			for id, cwd := range opts.ThreadCwdByID {
				if id == "" || strings.TrimSpace(cwd) == "" {
					continue
				}
				res, err := stmt.Exec(cwd, id, cwd)
				if err == nil {
					c, _ := res.RowsAffected()
					result.CwdRowsUpdated += int(c)
				}
			}
			stmt.Close()
		}
	}

	result.TotalUpdated = result.ProviderRowsUpdated + result.UserEventRowsUpdated + result.CwdRowsUpdated

	if afterUpdate != nil {
		if err := afterUpdate(result); err != nil {
			return result, err
		}
	}

	if err := tx.Commit(); err != nil {
		return result, err
	}
	committed = true

	return result, nil
}
