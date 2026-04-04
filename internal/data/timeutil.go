package data

import (
	"fmt"
	"time"
)

// SQLite 支持的 datetime 格式列表（从最完整到最简化）
var sqliteTimeFormats = []string{
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999Z07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// parseSQLiteTime 将 SQLite 存储的时间字符串解析为 time.Time。
// SQLite 以 TEXT 形式存储 DATETIME，格式通常为 "2006-01-02 15:04:05"。
func parseSQLiteTime(s string) (time.Time, error) {
	for _, layout := range sqliteTimeFormats {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("parseSQLiteTime: unrecognized format %q", s)
}

// formatSQLiteTime 将 time.Time 格式化为 SQLite 兼容的 datetime 字符串（UTC）。
func formatSQLiteTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}
