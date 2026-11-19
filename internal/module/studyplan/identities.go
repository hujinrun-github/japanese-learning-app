package studyplan

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrStudyTaskIdentityReused = errors.New("study task identity cannot be reused")

// KeepTaskIdentities prevents an already completed task from becoming a
// different task under the same identity. Retired identities survive removal
// until the learner explicitly creates a new plan generation.
func KeepTaskIdentities(ctx context.Context, tx *sql.Tx, uid int64, planID string, previous *Plan, next Plan) error {
	if previous != nil {
		for _, slot := range previous.TimeSlots {
			for _, task := range slot.Tasks {
				if _, err := tx.ExecContext(ctx, `INSERT INTO study_plan_task_identities(user_id,plan_id,slot_id,task_id,path) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, uid, planID, slot.ID, task.ID, resolvedTask(task).Path); err != nil {
					return fmt.Errorf("preserve study task identity: %w", err)
				}
			}
		}
	}
	active := make(map[string]bool)
	for _, slot := range next.TimeSlots {
		for _, task := range slot.Tasks {
			key := slot.ID + "\n" + task.ID
			active[key] = true
			path := resolvedTask(task).Path
			var existing string
			var retired bool
			err := tx.QueryRowContext(ctx, `SELECT path,retired FROM study_plan_task_identities WHERE user_id=$1 AND plan_id=$2 AND slot_id=$3 AND task_id=$4`, uid, planID, slot.ID, task.ID).Scan(&existing, &retired)
			if err == nil && (retired || existing != path) {
				return ErrStudyTaskIdentityReused
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("check study task identity: %w", err)
			}
			if errors.Is(err, sql.ErrNoRows) {
				if _, err := tx.ExecContext(ctx, `INSERT INTO study_plan_task_identities(user_id,plan_id,slot_id,task_id,path) VALUES($1,$2,$3,$4,$5)`, uid, planID, slot.ID, task.ID, path); err != nil {
					return fmt.Errorf("register study task identity: %w", err)
				}
			}
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT slot_id,task_id FROM study_plan_task_identities WHERE user_id=$1 AND plan_id=$2 AND retired=FALSE`, uid, planID)
	if err != nil {
		return fmt.Errorf("read live study identities: %w", err)
	}
	type identity struct{ slot, task string }
	var removed []identity
	for rows.Next() {
		var id identity
		if err := rows.Scan(&id.slot, &id.task); err != nil {
			rows.Close()
			return fmt.Errorf("scan live study identity: %w", err)
		}
		if !active[id.slot+"\n"+id.task] {
			removed = append(removed, id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("iterate live study identities: %w", err)
	}
	for _, id := range removed {
		if _, err := tx.ExecContext(ctx, `UPDATE study_plan_task_identities SET retired=TRUE WHERE user_id=$1 AND plan_id=$2 AND slot_id=$3 AND task_id=$4`, uid, planID, id.slot, id.task); err != nil {
			return fmt.Errorf("retire removed study identity: %w", err)
		}
	}
	return nil
}
