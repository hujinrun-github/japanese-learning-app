// Package flowspace delivers study-plan state to one explicitly paired
// FlowSpace workspace. Browser sessions never receive the signing credential.
package flowspace

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"japanese-learning-app/internal/module/studyplan"
)

type Options struct {
	BaseURL          string
	SourceInstanceID string
	MasterKey        []byte
	HTTPClient       *http.Client
	Now              func() time.Time
}

type Service struct {
	db       *sql.DB
	baseURL  string
	instance string
	aead     cipher.AEAD
	client   *http.Client
	now      func() time.Time
}

type EventStatus struct {
	EventID      string `json:"event_id"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	RemoteStatus string `json:"remote_status,omitempty"`
	Error        string `json:"error,omitempty"`
	CreatedAt    string `json:"created_at"`
}

type Status struct {
	Available      bool          `json:"available"`
	Connected      bool          `json:"connected"`
	ConnectionID   string        `json:"connection_id,omitempty"`
	WorkspaceID    string        `json:"workspace_id,omitempty"`
	WorkspaceName  string        `json:"workspace_name,omitempty"`
	PendingCount   int           `json:"pending_count"`
	DeliveredCount int           `json:"delivered_count"`
	LastError      string        `json:"last_error,omitempty"`
	Events         []EventStatus `json:"events"`
}

func OptionsFromEnv() (Options, error) {
	opts := Options{BaseURL: strings.TrimSpace(os.Getenv("FLOWSPACE_BASE_URL")), SourceInstanceID: strings.TrimSpace(os.Getenv("FLOWSPACE_SOURCE_INSTANCE_ID"))}
	if opts.BaseURL != "" {
		var err error
		opts.MasterKey, err = hex.DecodeString(strings.TrimSpace(os.Getenv("FLOWSPACE_INTEGRATION_MASTER_KEY")))
		if err != nil {
			return Options{}, fmt.Errorf("FLOWSPACE_INTEGRATION_MASTER_KEY must be 64 hexadecimal characters")
		}
	}
	return opts, nil
}

func NewService(db *sql.DB, opts Options) (*Service, error) {
	s := &Service{db: db, now: opts.Now, instance: opts.SourceInstanceID}
	if s.now == nil {
		s.now = time.Now
	}
	if opts.BaseURL == "" {
		return s, nil
	}
	u, err := url.Parse(opts.BaseURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("FLOWSPACE_BASE_URL must be an absolute server URL without credentials, query or fragment")
	}
	loopback := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, fmt.Errorf("FLOWSPACE_BASE_URL requires HTTPS (HTTP is allowed only on loopback)")
	}
	if opts.SourceInstanceID == "" || len(opts.SourceInstanceID) > 200 || strings.ContainsAny(opts.SourceInstanceID, "\r\n\t ") {
		return nil, fmt.Errorf("FLOWSPACE_SOURCE_INSTANCE_ID must identify this deployment")
	}
	if len(opts.MasterKey) != 32 {
		return nil, fmt.Errorf("FLOWSPACE_INTEGRATION_MASTER_KEY must contain exactly 32 bytes")
	}
	block, err := aes.NewCipher(opts.MasterKey)
	if err != nil {
		return nil, fmt.Errorf("initialize FlowSpace credential encryption: %w", err)
	}
	s.aead, err = cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize FlowSpace credential envelope: %w", err)
	}
	s.baseURL = strings.TrimRight(opts.BaseURL, "/")
	client := http.Client{Timeout: 15 * time.Second}
	if opts.HTTPClient != nil {
		client = *opts.HTTPClient
		if client.Timeout == 0 {
			client.Timeout = 15 * time.Second
		}
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	s.client = &client
	return s, nil
}

func (s *Service) encrypt(uid int64, connection, secret string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("create FlowSpace encryption nonce: %w", err)
	}
	aad := []byte(strconv.FormatInt(uid, 10) + "\n" + connection)
	return hex.EncodeToString(s.aead.Seal(nonce, nonce, []byte(secret), aad)), nil
}

func (s *Service) decrypt(uid int64, connection, encoded string) (string, error) {
	buf, err := hex.DecodeString(encoded)
	if err != nil || len(buf) < s.aead.NonceSize() {
		return "", errors.New("FlowSpace credential envelope is invalid")
	}
	n := s.aead.NonceSize()
	value, err := s.aead.Open(nil, buf[:n], buf[n:], []byte(strconv.FormatInt(uid, 10)+"\n"+connection))
	if err != nil {
		return "", errors.New("FlowSpace credential cannot be decrypted; verify the integration master key")
	}
	return string(value), nil
}

func (s *Service) Pair(ctx context.Context, uid int64, code string) (Status, error) {
	if s.baseURL == "" {
		return Status{}, errors.New("FlowSpace integration is not configured")
	}
	code = strings.TrimSpace(code)
	if len(code) < 16 || len(code) > 512 {
		return Status{}, errors.New("pairing code is invalid")
	}
	var connected bool
	err := s.db.QueryRowContext(ctx, `SELECT enabled FROM flowspace_connections WHERE user_id=$1`, uid).Scan(&connected)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Status{}, fmt.Errorf("read existing FlowSpace connection: %w", err)
	}
	if connected {
		return Status{}, errors.New("disconnect the current workspace before pairing another one")
	}
	request := map[string]string{"pairing_code": code, "source_instance_id": s.instance, "source_user_id": strconv.FormatInt(uid, 10)}
	raw, err := json.Marshal(request)
	if err != nil {
		return Status{}, fmt.Errorf("encode FlowSpace pairing: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/api/integrations/japanese/pair", bytes.NewReader(raw))
	if err != nil {
		return Status{}, fmt.Errorf("create FlowSpace pairing request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return Status{}, errors.New("FlowSpace pairing service is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Status{}, fmt.Errorf("FlowSpace pairing rejected (HTTP %d); create a new pairing code", resp.StatusCode)
	}
	var payload struct {
		Data struct {
			ConnectionID  string `json:"connection_id"`
			KeyID         string `json:"key_id"`
			Secret        string `json:"secret"`
			WorkspaceID   string `json:"workspace_id"`
			WorkspaceName string `json:"workspace_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&payload); err != nil {
		return Status{}, errors.New("FlowSpace pairing response is invalid")
	}
	p := payload.Data
	if p.ConnectionID == "" || p.KeyID == "" || p.Secret == "" || p.WorkspaceID == "" {
		return Status{}, errors.New("FlowSpace pairing response is incomplete")
	}
	encrypted, err := s.encrypt(uid, p.ConnectionID, p.Secret)
	if err != nil {
		return Status{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Status{}, fmt.Errorf("begin FlowSpace pairing: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO flowspace_connections(user_id,connection_id,key_id,source_instance_id,workspace_id,workspace_name,encrypted_secret) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(user_id) DO UPDATE SET connection_id=excluded.connection_id,key_id=excluded.key_id,source_instance_id=excluded.source_instance_id,workspace_id=excluded.workspace_id,workspace_name=excluded.workspace_name,encrypted_secret=excluded.encrypted_secret,enabled=TRUE,created_at=now() WHERE flowspace_connections.enabled=FALSE`, uid, p.ConnectionID, p.KeyID, s.instance, p.WorkspaceID, p.WorkspaceName, encrypted)
	if err != nil {
		return Status{}, fmt.Errorf("save FlowSpace pairing: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Status{}, fmt.Errorf("read FlowSpace pairing write: %w", err)
	}
	if n != 1 {
		return Status{}, errors.New("another FlowSpace pairing already completed")
	}
	if err := studyplan.QueueCatalog(ctx, tx, uid, false, s.now()); err != nil {
		return Status{}, err
	}
	if err := tx.Commit(); err != nil {
		return Status{}, fmt.Errorf("commit FlowSpace pairing: %w", err)
	}
	return s.Status(ctx, uid)
}

func (s *Service) Status(ctx context.Context, uid int64) (Status, error) {
	result := Status{Available: s.baseURL != "", Events: []EventStatus{}}
	if s.db == nil {
		return result, nil
	}
	err := s.db.QueryRowContext(ctx, `SELECT enabled,connection_id,workspace_id,workspace_name FROM flowspace_connections WHERE user_id=$1`, uid).Scan(&result.Connected, &result.ConnectionID, &result.WorkspaceID, &result.WorkspaceName)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return Status{}, fmt.Errorf("read FlowSpace status: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE status IN ('pending','sending','failed')),count(*) FILTER(WHERE status='delivered') FROM flowspace_outbox WHERE user_id=$1 AND connection_id=$2`, uid, result.ConnectionID).Scan(&result.PendingCount, &result.DeliveredCount); err != nil {
		return Status{}, fmt.Errorf("read FlowSpace queue counts: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_id,kind,status,remote_status,last_error,created_at FROM flowspace_outbox WHERE user_id=$1 AND connection_id=$2 ORDER BY sequence DESC LIMIT 10`, uid, result.ConnectionID)
	if err != nil {
		return Status{}, fmt.Errorf("read FlowSpace event status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var event EventStatus
		var created time.Time
		if err := rows.Scan(&event.EventID, &event.Kind, &event.Status, &event.RemoteStatus, &event.Error, &created); err != nil {
			return Status{}, fmt.Errorf("scan FlowSpace event status: %w", err)
		}
		event.CreatedAt = created.UTC().Format(time.RFC3339)
		result.Events = append(result.Events, event)
		if result.LastError == "" && event.Error != "" {
			result.LastError = event.Error
		}
	}
	if err := rows.Err(); err != nil {
		return Status{}, fmt.Errorf("iterate FlowSpace status: %w", err)
	}
	return result, nil
}

func (s *Service) Disconnect(ctx context.Context, uid int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin FlowSpace disconnect: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE flowspace_connections SET enabled=FALSE WHERE user_id=$1`, uid); err != nil {
		return fmt.Errorf("disable FlowSpace connection: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE flowspace_outbox SET status='cancelled',lease_token='',lease_until=NULL WHERE user_id=$1 AND status IN ('pending','sending','failed')`, uid); err != nil {
		return fmt.Errorf("cancel FlowSpace deliveries: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit FlowSpace disconnect: %w", err)
	}
	return nil
}

func (s *Service) Retry(ctx context.Context, uid int64) error {
	// Delivered events are refreshed by GET, never re-posted. This lets a
	// FlowSpace-side conflict repair become visible immediately after retry.
	if _, err := s.db.ExecContext(ctx, `UPDATE flowspace_outbox SET status=CASE WHEN status='failed' THEN 'pending' ELSE status END,last_error='',next_attempt_at=$2 WHERE user_id=$1 AND (status='failed' OR (status='delivered' AND kind='event' AND remote_status IN ('received','processing','retry_wait','pending','needs_attention'))) AND connection_id IN (SELECT connection_id FROM flowspace_connections WHERE user_id=$1 AND enabled=TRUE)`, uid, s.now()); err != nil {
		return fmt.Errorf("retry FlowSpace deliveries: %w", err)
	}
	return nil
}

func (s *Service) signedRequest(ctx context.Context, method, target, connection, key, secret string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+target, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create FlowSpace event request: %w", err)
	}
	nonce, err := studyplan.IntegrationID("")
	if err != nil {
		return nil, err
	}
	timestamp := strconv.FormatInt(s.now().Unix(), 10)
	sum := sha256.Sum256(body)
	canonical := strings.Join([]string{method, target, connection, key, timestamp, nonce, hex.EncodeToString(sum[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-FlowSpace-Connection", connection)
	req.Header.Set("X-FlowSpace-Key-Id", key)
	req.Header.Set("X-FlowSpace-Timestamp", timestamp)
	req.Header.Set("X-FlowSpace-Nonce", nonce)
	req.Header.Set("X-FlowSpace-Signature", hex.EncodeToString(mac.Sum(nil)))
	return req, nil
}
