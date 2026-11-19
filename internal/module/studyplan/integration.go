package studyplan

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

type CompletionEvent struct {
	SchemaVersion    int    `json:"schema_version"`
	EventID          string `json:"event_id"`
	Type             string `json:"type"`
	SourceInstanceID string `json:"source_instance_id"`
	SourceUserID     string `json:"source_user_id"`
	PlanID           string `json:"plan_id"`
	PlanRevision     int    `json:"plan_revision"`
	CatalogRevision  int64  `json:"catalog_revision"`
	SlotID           string `json:"slot_id"`
	TaskID           string `json:"task_id"`
	StudyDate        string `json:"study_date"`
	Timezone         string `json:"timezone"`
	StateRevision    int64  `json:"state_revision"`
	Completed        bool   `json:"completed"`
	OccurredAt       string `json:"occurred_at"`
	Reason           string `json:"reason"`
}

type CatalogPlan struct {
	PlanID    string     `json:"plan_id"`
	Revision  int        `json:"revision"`
	Title     string     `json:"title"`
	Timezone  string     `json:"timezone"`
	StartDate string     `json:"start_date"`
	EndDate   string     `json:"end_date"`
	Weekdays  []int      `json:"weekdays"`
	TimeSlots []TimeSlot `json:"time_slots"`
}

type Catalog struct {
	SchemaVersion    int          `json:"schema_version"`
	SourceInstanceID string       `json:"source_instance_id"`
	SourceUserID     string       `json:"source_user_id"`
	CatalogRevision  int64        `json:"catalog_revision"`
	Plan             *CatalogPlan `json:"plan"`
}

var ErrStudyDateChanged = errors.New("study date changed")
var ErrStudyTaskNotFound = errors.New("study task not found")

func IntegrationID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("create integration ID: %w", err)
	}
	return prefix + hex.EncodeToString(b[:]), nil
}

func catalogRevision(ctx context.Context, db TaskDB, uid int64, bump bool) (int64, error) {
	query := `INSERT INTO study_plan_catalog_versions(user_id,revision) VALUES($1,1) ON CONFLICT(user_id) DO UPDATE SET revision=study_plan_catalog_versions.revision RETURNING revision`
	if bump {
		query = `INSERT INTO study_plan_catalog_versions(user_id,revision) VALUES($1,1) ON CONFLICT(user_id) DO UPDATE SET revision=study_plan_catalog_versions.revision+1 RETURNING revision`
	}
	var revision int64
	if err := db.QueryRowContext(ctx, query, uid).Scan(&revision); err != nil {
		return 0, fmt.Errorf("resolve study catalog revision: %w", err)
	}
	return revision, nil
}

// QueueCatalog is called in the same transaction as a plan change, or during
// pairing without a bump. Its revision deliberately survives deleting a plan.
func QueueCatalog(ctx context.Context, db TaskDB, uid int64, bump bool, now time.Time) error {
	revision, err := catalogRevision(ctx, db, uid, bump)
	if err != nil {
		return err
	}
	var connection, instance string
	err = db.QueryRowContext(ctx, `SELECT connection_id,source_instance_id FROM flowspace_connections WHERE user_id=$1 AND enabled=TRUE`, uid).Scan(&connection, &instance)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read FlowSpace catalog connection: %w", err)
	}
	catalog := Catalog{SchemaVersion: 1, SourceInstanceID: instance, SourceUserID: strconv.FormatInt(uid, 10), CatalogRevision: revision}
	var raw, planID string
	var planRevision int
	err = db.QueryRowContext(ctx, `SELECT plan_json,plan_id,revision FROM study_plans WHERE user_id=$1`, uid).Scan(&raw, &planID, &planRevision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read study catalog plan: %w", err)
	}
	if err == nil {
		var p Plan
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return fmt.Errorf("decode study catalog plan: %w", err)
		}
		for i := range p.TimeSlots {
			for j := range p.TimeSlots[i].Tasks {
				p.TimeSlots[i].Tasks[j] = resolvedTask(p.TimeSlots[i].Tasks[j])
			}
		}
		catalog.Plan = &CatalogPlan{PlanID: planID, Revision: planRevision, Title: p.Title, Timezone: p.Timezone, StartDate: p.StartDate, EndDate: p.EndDate, Weekdays: p.Weekdays, TimeSlots: p.TimeSlots}
	}
	return enqueueIntegration(ctx, db, uid, connection, "catalog", catalog, now)
}

func enqueueIntegration(ctx context.Context, db TaskDB, uid int64, connection, kind string, payload any, now time.Time) error {
	id, err := IntegrationID("evt_")
	if err != nil {
		return err
	}
	if event, ok := payload.(CompletionEvent); ok {
		event.EventID = id
		payload = event
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode FlowSpace event: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO flowspace_outbox(event_id,user_id,connection_id,kind,payload_json,created_at,next_attempt_at) VALUES($1,$2,$3,$4,$5,$6,$6)`, id, uid, connection, kind, string(raw), now); err != nil {
		return fmt.Errorf("queue FlowSpace event: %w", err)
	}
	return nil
}

// SetTaskCompletion requires a caller-owned transaction. Locking the plan also
// serializes manual/listening writes and plan changes before version allocation.
func SetTaskCompletion(ctx context.Context, db TaskDB, uid int64, date, slotID, taskID string, completed bool, reason string, now time.Time) error {
	var raw, planID string
	var planRevision int
	if err := db.QueryRowContext(ctx, `SELECT plan_json,plan_id,revision FROM study_plans WHERE user_id=$1 FOR UPDATE`, uid).Scan(&raw, &planID, &planRevision); err != nil {
		return fmt.Errorf("lock study task plan: %w", err)
	}
	var p Plan
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return fmt.Errorf("decode completion plan: %w", err)
	}
	today, err := p.Today(now)
	if err != nil {
		return fmt.Errorf("resolve completion day: %w", err)
	}
	if today.Date != date {
		return ErrStudyDateChanged
	}
	found := false
	for _, slot := range today.Slots {
		if slot.ID == slotID {
			for _, task := range slot.Tasks {
				found = found || task.ID == taskID
			}
		}
	}
	if !found {
		return ErrStudyTaskNotFound
	}
	// The legacy completion row remains the current display authority, including
	// pre-integration records and records deliberately cleared by older clients.
	var result sql.Result
	if completed {
		result, err = db.ExecContext(ctx, `INSERT INTO study_plan_task_completions(user_id,study_date,slot_id,task_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, uid, date, slotID, taskID)
	} else {
		result, err = db.ExecContext(ctx, `DELETE FROM study_plan_task_completions WHERE user_id=$1 AND study_date=$2 AND slot_id=$3 AND task_id=$4`, uid, date, slotID, taskID)
	}
	if err != nil {
		return fmt.Errorf("save study task completion: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read study task completion effect: %w", err)
	}
	if n == 0 {
		return nil
	}
	var stateRevision int64
	if err := db.QueryRowContext(ctx, `INSERT INTO study_plan_task_states(user_id,plan_id,study_date,slot_id,task_id,completed,revision,occurred_at,reason) VALUES($1,$2,$3,$4,$5,$6,1,$7,$8) ON CONFLICT(user_id,plan_id,study_date,slot_id,task_id) DO UPDATE SET completed=excluded.completed,revision=study_plan_task_states.revision+1,occurred_at=excluded.occurred_at,reason=excluded.reason RETURNING revision`, uid, planID, date, slotID, taskID, completed, now, reason).Scan(&stateRevision); err != nil {
		return fmt.Errorf("save versioned study task state: %w", err)
	}
	revision, err := catalogRevision(ctx, db, uid, false)
	if err != nil {
		return err
	}
	var connection, instance string
	err = db.QueryRowContext(ctx, `SELECT connection_id,source_instance_id FROM flowspace_connections WHERE user_id=$1 AND enabled=TRUE`, uid).Scan(&connection, &instance)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read FlowSpace completion connection: %w", err)
	}
	event := CompletionEvent{SchemaVersion: 1, Type: "study_task.state_changed.v1", SourceInstanceID: instance, SourceUserID: strconv.FormatInt(uid, 10), PlanID: planID, PlanRevision: planRevision, CatalogRevision: revision, SlotID: slotID, TaskID: taskID, StudyDate: date, Timezone: p.Timezone, StateRevision: stateRevision, Completed: completed, OccurredAt: now.UTC().Format(time.RFC3339Nano), Reason: reason}
	return enqueueIntegration(ctx, db, uid, connection, "event", event, now)
}
