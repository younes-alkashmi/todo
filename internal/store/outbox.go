package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PendingMutation is the shape the future sync client will read.
type PendingMutation struct {
	MutationID  string
	Account     string
	EntityType  string
	EntityID    string
	BaseVersion int
	Operation   string
	Payload     json.RawMessage
	State       string
	CreatedAt   time.Time
}

func insertOutbox(ctx context.Context, tx *sql.Tx, account, entityType, entityID string, baseVersion int, op string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO outbox (mutation_id, account, entity_type, entity_id, base_version, operation, payload_json, state, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?)`,
		newMutationID(), account, entityType, entityID, baseVersion, op, string(body),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (r *TaskRepo) PendingMutations(ctx context.Context) ([]PendingMutation, error) {
	rows, err := r.s.DB.QueryContext(ctx,
		`SELECT mutation_id, account, entity_type, entity_id, base_version, operation, payload_json, state, created_at
		 FROM outbox WHERE state IN ('pending', 'conflict') ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingMutation
	for rows.Next() {
		var (
			m        PendingMutation
			payload  string
			createdS string
		)
		if err := rows.Scan(&m.MutationID, &m.Account, &m.EntityType, &m.EntityID,
			&m.BaseVersion, &m.Operation, &payload, &m.State, &createdS); err != nil {
			return nil, err
		}
		m.Payload = json.RawMessage(payload)
		if ts, err := time.Parse(time.RFC3339Nano, createdS); err == nil {
			m.CreatedAt = ts
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func newMutationID() string { return uuid.NewString() }

func (r *TaskRepo) MarkMutationSent(ctx context.Context, mutationID string) error {
	_, err := r.s.DB.ExecContext(ctx,
		`UPDATE outbox SET state = 'sent' WHERE mutation_id = ?`, mutationID)
	return err
}

func (r *TaskRepo) MarkConflict(ctx context.Context, mutationID string, serverPayload map[string]interface{}) error {
	payload, _ := json.Marshal(serverPayload)
	_, err := r.s.DB.ExecContext(ctx,
		`UPDATE outbox SET state = 'conflict', payload_json = ? WHERE mutation_id = ?`,
		string(payload), mutationID)
	return err
}

func (r *TaskRepo) MarkDenied(ctx context.Context, mutationID, reason string) error {
	_, err := r.s.DB.ExecContext(ctx,
		`UPDATE outbox SET state = 'failed' WHERE mutation_id = ?`, mutationID)
	return err
}
