package store

import "context"

type MetaRepo struct{ s *Store }

func (s *Store) Meta() *MetaRepo { return &MetaRepo{s: s} }

func (r *MetaRepo) Get(key string) (string, error) {
	var v string
	err := r.s.DB.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	return v, err
}

func (r *MetaRepo) Set(ctx context.Context, key, value string) error {
	_, err := r.s.DB.ExecContext(ctx,
		`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}
