package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/younes-alkashmi/todo/internal/domain"
)

var ErrNotFound = errors.New("not found")

type TaskRepo struct{ s *Store }

func (s *Store) Tasks() *TaskRepo { return &TaskRepo{s: s} }

// CreateTask inserts a task and its outbox mutation atomically.
// Returns the committed task (with generated ID, version 1, timestamps).
func (r *TaskRepo) CreateTask(ctx context.Context, t *domain.Task) (*domain.Task, error) {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.OwnerUID == "" {
		t.OwnerUID = "local"
	}
	t.Version = 1
	now := time.Now().UTC()
	t.CreatedAt, t.UpdatedAt = now, now

	tx, err := r.s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := insertTask(ctx, tx, t); err != nil {
		return nil, err
	}

	if err := insertOutbox(ctx, tx, t.OwnerUID, "task", t.ID, 0, "create", t); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTask replaces the mutable fields of an existing task and queues
// an outbox mutation. base_version is captured from the loaded task.
func (r *TaskRepo) UpdateTask(ctx context.Context, in *domain.Task) (*domain.Task, error) {
	cur, err := r.GetTask(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	baseVersion := cur.Version
	now := time.Now().UTC()
	cur.Title = in.Title
	cur.Description = in.Description
	cur.DueDate = in.DueDate
	cur.Tags = in.Tags
	cur.Completed = in.Completed
	cur.ParentID = in.ParentID
	cur.ProjectID = in.ProjectID
	cur.Version = baseVersion + 1
	cur.UpdatedAt = now

	tx, err := r.s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := updateTaskRow(ctx, tx, cur); err != nil {
		return nil, err
	}
	if err := insertOutbox(ctx, tx, cur.OwnerUID, "task", cur.ID, baseVersion, "update", cur); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return cur, nil
}

// DeleteTask soft-deletes a task and its children (cascade rule), plus
// queues an outbox mutation for each. US-04.
func (r *TaskRepo) DeleteTask(ctx context.Context, id string) (affected int, err error) {
	tx, err := r.s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Find task + children (one level).
	rows, err := tx.QueryContext(ctx,
		`SELECT id, version, owner_uid FROM tasks
		 WHERE deleted_at IS NULL AND (id = ? OR parent_id = ?)`,
		id, id,
	)
	if err != nil {
		return 0, err
	}
	type victim struct {
		id      string
		version int
		owner   string
	}
	var victims []victim
	for rows.Next() {
		if err := rows.Err(); err != nil {
			continue
		}
		var v victim
		if err := rows.Scan(&v.id, &v.version, &v.owner); err != nil {
			rows.Close()
			return 0, err
		}
		victims = append(victims, v)
	}
	rows.Close()
	if len(victims) == 0 {
		return 0, ErrNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, v := range victims {
		if _, err := tx.ExecContext(ctx,
			`UPDATE tasks SET deleted_at = ?, version = version + 1, updated_at = ?
			 WHERE id = ? AND deleted_at IS NULL`,
			now, now, v.id,
		); err != nil {
			return 0, err
		}
		if err := insertOutbox(ctx, tx, v.owner, "task", v.id, v.version, "delete", map[string]string{"id": v.id}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(victims), nil
}

// GetTask loads one non-deleted task.
func (r *TaskRepo) GetTask(ctx context.Context, id string) (*domain.Task, error) {
	row := r.s.DB.QueryRowContext(ctx, selectTaskSQL+` WHERE id = ? AND deleted_at IS NULL`, id)
	return scanTask(row)
}

// ListTasks returns non-deleted tasks, optionally scoped.
// scope: nil => all; "" => global only; "proj_xyz" => that project.
func (r *TaskRepo) ListTasks(ctx context.Context, projectID *string) ([]*domain.Task, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if projectID == nil {
		rows, err = r.s.DB.QueryContext(ctx, selectTaskSQL+` WHERE deleted_at IS NULL ORDER BY created_at`)
	} else if *projectID == "" {
		rows, err = r.s.DB.QueryContext(ctx, selectTaskSQL+` WHERE deleted_at IS NULL AND project_id IS NULL ORDER BY created_at`)
	} else {
		rows, err = r.s.DB.QueryContext(ctx, selectTaskSQL+` WHERE deleted_at IS NULL AND project_id = ? ORDER BY created_at`, *projectID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ---- helpers ----

const selectTaskSQL = `SELECT id, owner_uid, project_id, parent_id, title, description,
	due_date, tags_json, completed, version, created_at, updated_at, deleted_at FROM tasks`

type scanner interface{ Scan(...any) error }

func scanTask(sc scanner) (*domain.Task, error) {
	var (
		t                                       domain.Task
		projectID, parentID, dueDate, deletedAt sql.NullString
		tagsJSON                                string
		completed                               int
		createdAt, updatedAt                    string
	)
	err := sc.Scan(
		&t.ID, &t.OwnerUID, &projectID, &parentID, &t.Title, &t.Description,
		&dueDate, &tagsJSON, &completed, &t.Version, &createdAt, &updatedAt, &deletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if projectID.Valid {
		v := projectID.String
		t.ProjectID = &v
	}
	if parentID.Valid {
		v := parentID.String
		t.ParentID = &v
	}
	if dueDate.Valid && dueDate.String != "" {
		d, err := domain.ParseDate(dueDate.String)
		if err != nil {
			return nil, err
		}
		t.DueDate = &d
	}
	if err := json.Unmarshal([]byte(tagsJSON), &t.Tags); err != nil {
		return nil, err
	}
	t.Completed = completed != 0
	if ts, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
		t.CreatedAt = ts
	}
	if ts, err := time.Parse(time.RFC3339Nano, updatedAt); err == nil {
		t.UpdatedAt = ts
	}
	if deletedAt.Valid && deletedAt.String != "" {
		if ts, err := time.Parse(time.RFC3339Nano, deletedAt.String); err == nil {
			t.DeletedAt = &ts
		}
	}
	return &t, nil
}

func insertTask(ctx context.Context, tx *sql.Tx, t *domain.Task) error {
	tags, _ := json.Marshal(t.Tags)
	var due *string
	if t.DueDate != nil {
		s := t.DueDate.String()
		due = &s
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO tasks (id, owner_uid, project_id, parent_id, title, description,
			due_date, tags_json, completed, version, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.OwnerUID, t.ProjectID, t.ParentID, t.Title, t.Description,
		due, string(tags), boolToInt(t.Completed), t.Version,
		t.CreatedAt.Format(time.RFC3339Nano), t.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func updateTaskRow(ctx context.Context, tx *sql.Tx, t *domain.Task) error {
	tags, _ := json.Marshal(t.Tags)
	var due *string
	if t.DueDate != nil {
		s := t.DueDate.String()
		due = &s
	}
	_, err := tx.ExecContext(ctx,
		`UPDATE tasks SET project_id = ?, parent_id = ?, title = ?, description = ?,
			due_date = ?, tags_json = ?, completed = ?, version = ?, updated_at = ?
		 WHERE id = ? AND deleted_at IS NULL`,
		t.ProjectID, t.ParentID, t.Title, t.Description, due, string(tags),
		boolToInt(t.Completed), t.Version,
		t.UpdatedAt.Format(time.RFC3339Nano), t.ID,
	)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (r *TaskRepo) ApplyRemoteChanges(ctx context.Context, changes []map[string]interface{}, nextCursor string) error {
	tx, err := r.s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, doc := range changes {
		// Skip if there's a pending local mutation for this entity (conflict protection)
		id, _ := doc["id"].(string)
		if id == "" {
			continue
		}
		var pending int
		_ = tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM outbox WHERE entity_id = ? AND state IN ('pending','conflict')`,
			id).Scan(&pending)
		if pending > 0 {
			continue
		}

		// Safely extract fields with defaults
		title, _ := doc["title"].(string)
		if title == "" {
			title = "(untitled)" // fallback to satisfy NOT NULL
		}
		description, _ := doc["description"].(string)

		var dueDate interface{}
		if v, ok := doc["dueDate"]; ok && v != nil {
			dueDate = v
		}

		var tagsJSON string
		if tags, ok := doc["tags"].([]interface{}); ok {
			b, _ := json.Marshal(tags)
			tagsJSON = string(b)
		} else {
			tagsJSON = "[]"
		}

		completed := 0
		if v, ok := doc["completed"].(bool); ok && v {
			completed = 1
		}

		var version int64
		switch v := doc["version"].(type) {
		case int64:
			version = v
		case float64:
			version = int64(v)
		}

		var createdAt, updatedAt string
		if v, ok := doc["createdAt"].(string); ok {
			createdAt = v
		}
		if v, ok := doc["updatedAt"].(string); ok {
			updatedAt = v
		}

		var projectID, parentID interface{}
		if v, ok := doc["projectId"]; ok && v != nil {
			projectID = v
		}
		if v, ok := doc["parentId"]; ok && v != nil {
			parentID = v
		}

		_, err := tx.ExecContext(ctx, `
			INSERT INTO tasks (id, owner_uid, project_id, parent_id, title, description, due_date, tags_json, completed, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				project_id = excluded.project_id,
				parent_id = excluded.parent_id,
				title = excluded.title,
				description = excluded.description,
				due_date = excluded.due_date,
				tags_json = excluded.tags_json,
				completed = excluded.completed,
				version = excluded.version,
				updated_at = excluded.updated_at`,
			id, "local", projectID, parentID, title, description,
			dueDate, tagsJSON, completed, version, createdAt, updatedAt)
		if err != nil {
			return err
		}
	}

	// Persist cursor in the same transaction
	_, err = tx.ExecContext(ctx,
		`INSERT INTO meta(key, value) VALUES('sync_cursor', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, nextCursor)
	if err != nil {
		return err
	}
	return tx.Commit()
}
