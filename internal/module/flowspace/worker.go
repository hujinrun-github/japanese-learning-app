package flowspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"japanese-learning-app/internal/module/studyplan"
)

type delivery struct {
	id         string
	uid        int64
	connection string
	kind       string
	payload    string
	attempts   int
	lease      string
}

// Run recovers durable leases after restarts and never performs HTTP inside a
// study transaction. Cancellation is joined by the server before closing DB.
func (s *Service) Run(ctx context.Context) {
	if s.baseURL == "" {
		return
	}
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		for n := 0; n < 25; n++ {
			worked, err := s.RunOnce(ctx)
			if err != nil && ctx.Err() == nil {
				slog.Warn("FlowSpace delivery deferred", "error", err)
			}
			if !worked || ctx.Err() != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

// RunOnce is also the deterministic integration-test entry point. A 202 marks
// durable delivery only; a later signed GET obtains the processing receipt.
func (s *Service) RunOnce(ctx context.Context) (bool, error) {
	if s.baseURL == "" {
		return false, nil
	}
	lease, err := studyplan.IntegrationID("lease_")
	if err != nil {
		return false, err
	}
	var job delivery
	err = s.db.QueryRowContext(ctx, `WITH candidate AS (
 SELECT o.event_id FROM flowspace_outbox o
 JOIN flowspace_connections c ON c.user_id=o.user_id AND c.connection_id=o.connection_id
 WHERE c.enabled=TRUE AND c.source_instance_id=$1
 AND ((o.status='pending' AND o.next_attempt_at<=$2) OR (o.status='sending' AND o.lease_until<=$2))
 AND NOT EXISTS (SELECT 1 FROM flowspace_outbox older WHERE older.user_id=o.user_id AND older.connection_id=o.connection_id AND older.sequence<o.sequence AND older.status IN ('pending','sending','failed'))
 ORDER BY o.sequence FOR UPDATE OF o SKIP LOCKED LIMIT 1
) UPDATE flowspace_outbox o SET status='sending',attempts=o.attempts+1,lease_token=$3,lease_until=$4
 FROM candidate WHERE o.event_id=candidate.event_id
 RETURNING o.event_id,o.user_id,o.connection_id,o.kind,o.payload_json,o.attempts,o.lease_token`, s.instance, s.now(), lease, s.now().Add(time.Minute)).Scan(&job.id, &job.uid, &job.connection, &job.kind, &job.payload, &job.attempts, &job.lease)
	if errors.Is(err, sql.ErrNoRows) {
		return s.pollOnce(ctx, lease)
	}
	if err != nil {
		return false, fmt.Errorf("claim FlowSpace delivery: %w", err)
	}
	key, secret, err := s.credential(ctx, job.uid, job.connection)
	if err != nil {
		return true, s.failDelivery(ctx, job, err, false)
	}
	target := "/api/integrations/japanese/events"
	method := http.MethodPost
	if job.kind == "catalog" {
		target = "/api/integrations/japanese/catalog"
		method = http.MethodPut
	}
	req, err := s.signedRequest(ctx, method, target, job.connection, key, secret, []byte(job.payload))
	if err != nil {
		return true, s.failDelivery(ctx, job, err, false)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return true, s.failDelivery(ctx, job, errors.New("FlowSpace event delivery unavailable"), true)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryable := resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode >= 500
		return true, s.failDelivery(ctx, job, fmt.Errorf("FlowSpace rejected delivery (HTTP %d)", resp.StatusCode), retryable)
	}
	remoteStatus := "applied"
	if job.kind == "event" {
		remoteStatus = "received"
	}
	var payload struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&payload); err == nil && payload.Data.Status != "" {
		remoteStatus = payload.Data.Status
	}
	refreshDelay := 5 * time.Second
	if remoteStatus == "needs_attention" {
		refreshDelay = time.Minute
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE flowspace_outbox SET status='delivered',remote_status=$3,last_error='',delivered_at=$4,lease_token='',lease_until=NULL,next_attempt_at=$5 WHERE event_id=$1 AND lease_token=$2 AND status='sending'`, job.id, job.lease, remoteStatus, s.now(), s.now().Add(refreshDelay)); err != nil {
		return true, fmt.Errorf("record FlowSpace delivery receipt: %w", err)
	}
	return true, nil
}

func (s *Service) credential(ctx context.Context, uid int64, connection string) (string, string, error) {
	var key, encoded string
	if err := s.db.QueryRowContext(ctx, `SELECT key_id,encrypted_secret FROM flowspace_connections WHERE user_id=$1 AND connection_id=$2 AND enabled=TRUE AND source_instance_id=$3`, uid, connection, s.instance).Scan(&key, &encoded); err != nil {
		return "", "", errors.New("FlowSpace connection is unavailable")
	}
	secret, err := s.decrypt(uid, connection, encoded)
	return key, secret, err
}

func (s *Service) failDelivery(ctx context.Context, job delivery, cause error, retryable bool) error {
	state := "failed"
	if retryable {
		state = "pending"
	}
	delay := 5 * time.Second
	for i := 1; i < job.attempts && delay < 5*time.Minute; i++ {
		delay *= 2
	}
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE flowspace_outbox SET status=$3,last_error=$4,next_attempt_at=$5,lease_token='',lease_until=NULL WHERE event_id=$1 AND lease_token=$2 AND status='sending'`, job.id, job.lease, state, cause.Error(), s.now().Add(delay)); err != nil {
		return fmt.Errorf("persist FlowSpace retry: %w", err)
	}
	return cause
}

func (s *Service) pollOnce(ctx context.Context, lease string) (bool, error) {
	var job delivery
	err := s.db.QueryRowContext(ctx, `WITH candidate AS (
 SELECT o.event_id FROM flowspace_outbox o JOIN flowspace_connections c ON c.user_id=o.user_id AND c.connection_id=o.connection_id
 WHERE c.enabled=TRUE AND c.source_instance_id=$1 AND o.kind='event' AND o.status='delivered'
 AND o.remote_status IN ('received','processing','retry_wait','pending','needs_attention') AND o.next_attempt_at<=$2 AND (o.lease_until IS NULL OR o.lease_until<=$2)
 ORDER BY o.sequence FOR UPDATE OF o SKIP LOCKED LIMIT 1
) UPDATE flowspace_outbox o SET lease_token=$3,lease_until=$4 FROM candidate WHERE o.event_id=candidate.event_id
 RETURNING o.event_id,o.user_id,o.connection_id,o.lease_token`, s.instance, s.now(), lease, s.now().Add(time.Minute)).Scan(&job.id, &job.uid, &job.connection, &job.lease)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim FlowSpace receipt poll: %w", err)
	}
	key, secret, err := s.credential(ctx, job.uid, job.connection)
	status := ""
	failure := ""
	if err != nil {
		failure = err.Error()
	} else {
		req, requestErr := s.signedRequest(ctx, http.MethodGet, "/api/integrations/japanese/events/"+url.PathEscape(job.id), job.connection, key, secret, nil)
		if requestErr != nil {
			failure = "FlowSpace receipt request failed"
		} else {
			resp, httpErr := s.client.Do(req)
			if httpErr != nil {
				failure = "FlowSpace receipt service unavailable"
			} else {
				defer resp.Body.Close()
				if resp.StatusCode != 200 {
					failure = fmt.Sprintf("FlowSpace receipt unavailable (HTTP %d)", resp.StatusCode)
				} else {
					var payload struct {
						Data struct {
							Status    string `json:"status"`
							ErrorCode string `json:"error_code"`
							Error     string `json:"error"`
						} `json:"data"`
					}
					if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&payload); err != nil || payload.Data.Status == "" {
						failure = "FlowSpace receipt response invalid"
					} else {
						status = payload.Data.Status
						if status == "needs_attention" {
							failure = payload.Data.ErrorCode
							if failure == "" {
								failure = "FlowSpace task needs attention"
							}
						}
					}
				}
			}
		}
	}
	refreshDelay := 15 * time.Second
	if status == "needs_attention" || failure != "" {
		refreshDelay = time.Minute
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE flowspace_outbox SET remote_status=CASE WHEN $3='' THEN remote_status ELSE $3 END,last_error=$4,next_attempt_at=$5,lease_token='',lease_until=NULL WHERE event_id=$1 AND lease_token=$2 AND status='delivered'`, job.id, job.lease, status, failure, s.now().Add(refreshDelay)); err != nil {
		return true, fmt.Errorf("record FlowSpace processing status: %w", err)
	}
	return true, nil
}
