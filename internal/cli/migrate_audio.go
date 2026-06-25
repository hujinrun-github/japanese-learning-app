package cli

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type audioMigrateConfig struct {
	AudioDir      string
	MinIOEndpoint string
	MinIOBucket   string
	AccessKey     string
	SecretKey     string
	UseSSL        bool
	DryRun        bool
}

// migrateAudioFiles scans local audio files, uploads them to MinIO, creates audio_objects
// records in PostgreSQL, and updates foreign key references in migrated data.
func migrateAudioFiles(ctx context.Context, sqliteDB *sql.DB, pgDB *sql.DB, cfg audioMigrateConfig, stats *migrateStats) error {
	fmt.Println("\n--- Audio Migration ---")

	if cfg.DryRun {
		fmt.Println("[DRY RUN] Would scan audio files and migrate to MinIO")
	}

	// Initialize MinIO client
	minioClient, err := minio.New(cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return fmt.Errorf("create MinIO client: %w", err)
	}

	// Ensure bucket exists
	if !cfg.DryRun {
		exists, err := minioClient.BucketExists(ctx, cfg.MinIOBucket)
		if err != nil {
			return fmt.Errorf("check bucket %s: %w", cfg.MinIOBucket, err)
		}
		if !exists {
			if err := minioClient.MakeBucket(ctx, cfg.MinIOBucket, minio.MakeBucketOptions{}); err != nil {
				return fmt.Errorf("create bucket %s: %w", cfg.MinIOBucket, err)
			}
			fmt.Printf("  Created bucket: %s\n", cfg.MinIOBucket)
		}
	}

	audioStats := &audioStats{}
	stats.Audio = audioStats

	// Scan audio directories: words/ and examples/
	dirs := []struct {
		dir  string
		kind string
	}{
		{filepath.Join(cfg.AudioDir, "words"), "word_pronunciation"},
		{filepath.Join(cfg.AudioDir, "examples"), "example_audio"},
	}

	// Map: object_key -> audio_object_id (for FK updating later)
	audioObjectMap := make(map[string]int64)

	for _, d := range dirs {
		if err := processAudioDir(ctx, minioClient, pgDB, d.dir, d.kind, cfg, audioStats, audioObjectMap); err != nil {
			slog.Warn("migrateAudioFiles: error processing directory", "dir", d.dir, "err", err)
			stats.addError(fmt.Sprintf("audio dir %s: %v", d.dir, err))
		}
	}

	// Also scan the root audio directory for lesson-level .wav files
	lessonDir := cfg.AudioDir
	if err := processLessonAudioDir(ctx, minioClient, pgDB, lessonDir, cfg, audioStats, audioObjectMap); err != nil {
		slog.Warn("migrateAudioFiles: error processing lesson audio", "dir", lessonDir, "err", err)
		stats.addError(fmt.Sprintf("lesson audio dir: %v", err))
	}

	// Update FK references
	if err := updateAudioFKs(ctx, pgDB, sqliteDB, audioObjectMap, cfg, audioStats); err != nil {
		return fmt.Errorf("update audio FKs: %w", err)
	}

	return nil
}

// processAudioDir scans a directory of .wav files and uploads them to MinIO.
func processAudioDir(ctx context.Context, client *minio.Client, pgDB *sql.DB, dir, kind string, cfg audioMigrateConfig, stats *audioStats, objectMap map[string]int64) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("  Directory not found, skipping: %s\n", dir)
			return nil
		}
		return fmt.Errorf("read dir %s: %w", dir, err)
	}

	prefix := filepath.Base(dir) + "/" // "words/" or "examples/"

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".wav") {
			continue
		}

		stats.FilesScanned++
		filePath := filepath.Join(dir, entry.Name())

		// Compute SHA-256
		sha256Hex, size, err := computeFileSHA256(filePath)
		if err != nil {
			slog.Warn("processAudioDir: skipping file, cannot compute hash", "file", filePath, "err", err)
			stats.UploadedFail++
			continue
		}

		objectKey := prefix + entry.Name()

		if cfg.DryRun {
			fmt.Printf("  [DRY RUN] Would upload: %s -> %s/%s (sha256=%s, size=%d)\n", filePath, cfg.MinIOBucket, objectKey, sha256Hex, size)
			continue
		}

		// Upload to MinIO
		_, err = client.FPutObject(ctx, cfg.MinIOBucket, objectKey, filePath, minio.PutObjectOptions{
			ContentType: "audio/wav",
		})
		if err != nil {
			slog.Error("processAudioDir: upload failed", "file", filePath, "object", objectKey, "err", err)
			stats.UploadedFail++
			continue
		}
		stats.UploadedOK++

		// Create audio_objects record
		var objID int64
		err = pgDB.QueryRowContext(ctx,
			`INSERT INTO audio_objects (bucket, object_key, kind, visibility, content_sha256, size_bytes, mime_type)
			 VALUES ($1, $2, $3, 'public', $4, $5, 'audio/wav')
			 ON CONFLICT (bucket, object_key) DO UPDATE
			   SET content_sha256 = EXCLUDED.content_sha256,
			       size_bytes = EXCLUDED.size_bytes,
			       updated_at = now()
			 RETURNING id`,
			cfg.MinIOBucket, objectKey, kind, sha256Hex, size,
		).Scan(&objID)
		if err != nil {
			slog.Error("processAudioDir: insert audio_objects failed", "object_key", objectKey, "err", err)
			stats.UploadedFail++
			continue
		}

		objectMap[objectKey] = objID
	}

	return nil
}

// processLessonAudioDir scans the root audio directory and lessons/ for lesson-level .wav files.
func processLessonAudioDir(ctx context.Context, client *minio.Client, pgDB *sql.DB, dir string, cfg audioMigrateConfig, stats *audioStats, objectMap map[string]int64) error {
	if err := processLessonAudioFilesInDir(ctx, client, pgDB, dir, "", cfg, stats, objectMap); err != nil {
		return err
	}
	return processLessonAudioFilesInDir(ctx, client, pgDB, filepath.Join(dir, "lessons"), "lessons/", cfg, stats, objectMap)
}

func processLessonAudioFilesInDir(ctx context.Context, client *minio.Client, pgDB *sql.DB, dir, objectPrefix string, cfg audioMigrateConfig, stats *audioStats, objectMap map[string]int64) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".wav") {
			continue
		}

		stats.FilesScanned++
		filePath := filepath.Join(dir, entry.Name())

		sha256Hex, size, err := computeFileSHA256(filePath)
		if err != nil {
			slog.Warn("processLessonAudioDir: skipping file", "file", filePath, "err", err)
			stats.UploadedFail++
			continue
		}

		objectKey := objectPrefix + entry.Name() // top-level "server.wav" or "lessons/server.wav"

		if cfg.DryRun {
			fmt.Printf("  [DRY RUN] Would upload: %s -> %s/%s\n", filePath, cfg.MinIOBucket, objectKey)
			continue
		}

		_, err = client.FPutObject(ctx, cfg.MinIOBucket, objectKey, filePath, minio.PutObjectOptions{
			ContentType: "audio/wav",
		})
		if err != nil {
			slog.Error("processLessonAudioDir: upload failed", "file", filePath, "err", err)
			stats.UploadedFail++
			continue
		}
		stats.UploadedOK++

		var objID int64
		err = pgDB.QueryRowContext(ctx,
			`INSERT INTO audio_objects (bucket, object_key, kind, visibility, content_sha256, size_bytes, mime_type)
			 VALUES ($1, $2, 'lesson_audio', 'public', $3, $4, 'audio/wav')
			 ON CONFLICT (bucket, object_key) DO UPDATE
			   SET content_sha256 = EXCLUDED.content_sha256, size_bytes = EXCLUDED.size_bytes, updated_at = now()
			 RETURNING id`,
			cfg.MinIOBucket, objectKey, sha256Hex, size,
		).Scan(&objID)
		if err != nil {
			slog.Error("processLessonAudioDir: insert audio_objects failed", "object_key", objectKey, "err", err)
			stats.UploadedFail++
			continue
		}

		objectMap[objectKey] = objID
	}

	return nil
}

// updateAudioFKs scans the migrated data and updates audio_object_id foreign keys
// based on matching object_keys derived from SQLite's audio_url values.
func updateAudioFKs(ctx context.Context, pgDB *sql.DB, sqliteDB *sql.DB, objectMap map[string]int64, cfg audioMigrateConfig, stats *audioStats) error {
	if cfg.DryRun {
		fmt.Println("  [DRY RUN] Would update audio FKs")
		return nil
	}

	if len(objectMap) == 0 {
		fmt.Println("  No audio objects to link, skipping FK updates.")
		return nil
	}

	// Historical word audio can live under words/ or examples/.
	updated, err := updateTableAudioFKs(ctx, pgDB, sqliteDB, "words", "audio_url", "audio_object_id", "words/", "examples/")
	if err != nil {
		return fmt.Errorf("update words audio FK: %w", err)
	}
	stats.FKsUpdated += updated

	// Update speaking_materials: audio_url -> audio_url_legacy
	// object_key = "examples/{filename}"
	updated, err = updateTableAudioFKs(ctx, pgDB, sqliteDB, "speaking_materials", "audio_url", "audio_object_id", "examples/")
	if err != nil {
		return fmt.Errorf("update speaking_materials audio FK: %w", err)
	}
	stats.FKsUpdated += updated

	// Update lessons: audio_url could be full path like "/audio/lessons/xxx.wav" or just filename
	updated, err = updateLessonAudioFKs(ctx, pgDB, sqliteDB)
	if err != nil {
		return fmt.Errorf("update lessons audio FK: %w", err)
	}
	stats.FKsUpdated += updated

	updated, err = updateExampleAudioFKs(ctx, pgDB, "word_examples")
	if err != nil {
		return fmt.Errorf("update word_examples audio FK: %w", err)
	}
	stats.FKsUpdated += updated

	updated, err = updateExampleAudioFKs(ctx, pgDB, "grammar_examples")
	if err != nil {
		return fmt.Errorf("update grammar_examples audio FK: %w", err)
	}
	stats.FKsUpdated += updated

	return nil
}

// updateTableAudioFKs updates audio_object_id for a table using filename matches.
func updateTableAudioFKs(ctx context.Context, pgDB *sql.DB, sqliteDB *sql.DB, table, urlCol, fkCol string, prefixes ...string) (int, error) {
	// Query SQLite for audio_url values
	rows, err := sqliteDB.Query(fmt.Sprintf("SELECT id, %s FROM %s WHERE %s != ''", urlCol, table, urlCol))
	if err != nil {
		if strings.Contains(err.Error(), "no such column") {
			return 0, nil
		}
		return 0, fmt.Errorf("query %s.%s: %w", table, urlCol, err)
	}
	defer rows.Close()

	type mapping struct {
		id       int64
		audioURL string
	}
	var mappings []mapping
	for rows.Next() {
		var m mapping
		if err := rows.Scan(&m.id, &m.audioURL); err != nil {
			return 0, fmt.Errorf("scan %s: %w", table, err)
		}
		mappings = append(mappings, m)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("rows %s: %w", table, err)
	}

	updated := 0
	for _, m := range mappings {
		// Extract filename from the audio_url
		filename := filepath.Base(m.audioURL)

		// Find the audio_object_id
		var (
			objID int64
			found bool
		)
		for _, prefix := range prefixes {
			candidate := prefix + filename
			err := pgDB.QueryRowContext(ctx,
				`SELECT id FROM audio_objects WHERE object_key = $1 LIMIT 1`, candidate,
			).Scan(&objID)
			if err != nil {
				if err == sql.ErrNoRows {
					continue
				}
				return 0, fmt.Errorf("lookup audio_object for %s: %w", candidate, err)
			}
			found = true
			break
		}
		if !found {
			slog.Debug("updateTableAudioFKs: no audio_object for", "filename", filename, "prefixes", prefixes)
			continue
		}

		_, err = pgDB.ExecContext(ctx,
			fmt.Sprintf(`UPDATE %s SET %s = $1 WHERE id = $2`, table, fkCol),
			objID, m.id,
		)
		if err != nil {
			return 0, fmt.Errorf("update %s id=%d: %w", table, m.id, err)
		}
		updated++
	}

	return updated, nil
}

func updateExampleAudioFKs(ctx context.Context, pgDB *sql.DB, table string) (int, error) {
	rows, err := pgDB.QueryContext(ctx, fmt.Sprintf(`SELECT id, japanese FROM %s WHERE japanese != ''`, table))
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, fmt.Errorf("query %s.japanese: %w", table, err)
	}
	defer rows.Close()

	type mapping struct {
		id       int64
		japanese string
	}
	var mappings []mapping
	for rows.Next() {
		var m mapping
		if err := rows.Scan(&m.id, &m.japanese); err != nil {
			return 0, fmt.Errorf("scan %s: %w", table, err)
		}
		mappings = append(mappings, m)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("rows %s: %w", table, err)
	}

	updated := 0
	for _, m := range mappings {
		sum := sha256.Sum256([]byte(m.japanese))
		objectKey := "examples/" + fmt.Sprintf("%x", sum)[:16] + ".wav"

		var objID int64
		err := pgDB.QueryRowContext(ctx,
			`SELECT id FROM audio_objects WHERE object_key = $1 LIMIT 1`, objectKey,
		).Scan(&objID)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return 0, fmt.Errorf("lookup audio_object for %s: %w", objectKey, err)
		}

		_, err = pgDB.ExecContext(ctx,
			fmt.Sprintf(`UPDATE %s SET audio_object_id = $1 WHERE id = $2`, table),
			objID, m.id,
		)
		if err != nil {
			return 0, fmt.Errorf("update %s id=%d: %w", table, m.id, err)
		}
		updated++
	}

	return updated, nil
}

// updateLessonAudioFKs handles lessons specifically because audio_url may be a full path.
func updateLessonAudioFKs(ctx context.Context, pgDB *sql.DB, sqliteDB *sql.DB) (int, error) {
	rows, err := sqliteDB.Query(`SELECT id, audio_url FROM lessons WHERE audio_url != ''`)
	if err != nil {
		return 0, fmt.Errorf("query lessons.audio_url: %w", err)
	}
	defer rows.Close()

	type mapping struct {
		id       int64
		audioURL string
	}
	var mappings []mapping
	for rows.Next() {
		var m mapping
		if err := rows.Scan(&m.id, &m.audioURL); err != nil {
			return 0, fmt.Errorf("scan lessons: %w", err)
		}
		mappings = append(mappings, m)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("rows lessons: %w", err)
	}

	updated := 0
	for _, m := range mappings {
		// Extract filename from path (e.g. "/audio/lessons/server.wav" -> "server.wav")
		filename := filepath.Base(m.audioURL)

		// Try supported lesson audio locations.
		var objID int64
		err := pgDB.QueryRowContext(ctx,
			`SELECT id FROM audio_objects WHERE object_key = $1 OR object_key = $2 OR object_key = $3 LIMIT 1`,
			filename, "lessons/"+filename, "examples/"+filename,
		).Scan(&objID)
		if err != nil {
			if err == sql.ErrNoRows {
				slog.Debug("updateLessonAudioFKs: no audio_object for", "filename", filename)
				continue
			}
			return 0, fmt.Errorf("lookup audio_object for lesson %s: %w", filename, err)
		}

		_, err = pgDB.ExecContext(ctx, `UPDATE lessons SET audio_object_id = $1 WHERE id = $2`, objID, m.id)
		if err != nil {
			return 0, fmt.Errorf("update lesson id=%d: %w", m.id, err)
		}
		updated++
	}

	return updated, nil
}

// computeFileSHA256 computes the SHA-256 hash of a file and returns the hex string and size.
func computeFileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), size, nil
}
