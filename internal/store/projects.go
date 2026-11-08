package store

import (
	"context"
	"database/sql"
	// "errors"
	"time"

	"github.com/google/uuid"
	"github.com/younes-alkashmi/todo/internal/domain"
)

type ProjectRepo struct{ s *Store }

func (s *Store) Projects() *ProjectRepo { return &ProjectRepo{s: s} }

func (r *ProjectRepo) Create(ctx context.Context, p *domain.Project) (*domain.Project, error) {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.OwnerUID == "" {
		p.OwnerUID = "local"
	}
	p.Version = 1
	p.CreatedAt = time.Now().UTC()
	p.UpdatedAt = p.CreatedAt

	tx, err := r.s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO projects (id, owner_uid, name, version, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, p.OwnerUID, p.Name, p.Version,
		p.CreatedAt.Format(time.RFC3339Nano), p.UpdatedAt.Format(time.RFC3339Nano),
	); err != nil {
		return nil, err
	}
	if err := insertOutbox(ctx, tx, p.OwnerUID, "project", p.ID, 0, "create", p); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *ProjectRepo) List(ctx context.Context) ([]*domain.Project, error) {
	rows, err := r.s.DB.QueryContext(ctx,
		`SELECT id, owner_uid, name, version, created_at, updated_at
		 FROM projects WHERE deleted_at IS NULL ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Project
	for rows.Next() {
		var (
			p              domain.Project
			createdS, updS string
		)
		if err := rows.Scan(&p.ID, &p.OwnerUID, &p.Name, &p.Version, &createdS, &updS); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdS)
		p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updS)
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (r *ProjectRepo) Delete(ctx context.Context, id string) (int, error) {
	tx, err := r.s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)

	res, err := tx.ExecContext(ctx,
		`UPDATE projects SET deleted_at = ?, version = version + 1, updated_at = ?
		 WHERE id = ? AND deleted_at IS NULL`,
		now, now, id,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return 0, ErrNotFound
	}

	taskRes, err := tx.ExecContext(ctx,
		`UPDATE tasks SET deleted_at = ?, version = version + 1, updated_at = ?
		 WHERE project_id = ? AND deleted_at IS NULL`,
		now, now, id,
	)
	if err != nil {
		return 0, err
	}
	tasksAffected, _ := taskRes.RowsAffected()

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(tasksAffected), nil
}

var _ = sql.ErrNoRows // keep import if unused in your build; delete when real
