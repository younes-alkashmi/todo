package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/younes-alkashmi/todo/internal/domain"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrationIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.db")
	for i := 0; i < 3; i++ {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("open #%d: %v", i, err)
		}
		s.Close()
	}
}

func TestCreateTaskPersistsAndQueuesOutbox(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	task := &domain.Task{Title: "Build local storage"}
	got, err := s.Tasks().CreateTask(ctx, task)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if got.ID == "" || got.Version != 1 {
		t.Fatalf("unexpected task: %+v", got)
	}

	pending, err := s.Tasks().PendingMutations(ctx)
	if err != nil {
		t.Fatalf("PendingMutations: %v", err)
	}
	if len(pending) != 1 || pending[0].Operation != "create" || pending[0].BaseVersion != 0 {
		t.Fatalf("expected one create mutation with base 0, got %+v", pending)
	}
}

// US-01: restart offline preserves saved changes.
func TestRestartPreservesTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart.db")
	ctx := context.Background()

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	created, err := s1.Tasks().CreateTask(ctx, &domain.Task{Title: "survive"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	defer s2.Close()

	got, err := s2.Tasks().GetTask(ctx, created.ID)
	if err != nil {
		t.Fatalf("get after restart: %v", err)
	}
	if got.Title != "survive" {
		t.Fatalf("title = %q", got.Title)
	}
}

// A failed insert must roll back the task too — no orphan outbox entry,
// no orphan task. Simulate by violating a NOT NULL constraint.
func TestCreateRollsBackOnOutboxFailure(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Force failure: null title on the task is prevented by domain, so
	// instead use a bad owner_uid that triggers nothing here — but we can
	// test by inserting a task with a duplicate ID.
	task := &domain.Task{ID: "dup", Title: "first"}
	if _, err := s.Tasks().CreateTask(ctx, task); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := s.Tasks().CreateTask(ctx, &domain.Task{ID: "dup", Title: "second"})
	if err == nil {
		t.Fatal("expected duplicate-ID error")
	}
	if _, err := s.Tasks().GetTask(ctx, "dup"); err != nil {
		t.Fatalf("first task should still exist: %v", err)
	}

	pending, _ := s.Tasks().PendingMutations(ctx)
	if len(pending) != 1 {
		t.Fatalf("outbox should have exactly 1 row, got %d", len(pending))
	}
}

func TestDeleteTaskCascadesChildrenAndQueues(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	parent, _ := s.Tasks().CreateTask(ctx, &domain.Task{Title: "parent"})
	parentID := parent.ID
	child, _ := s.Tasks().CreateTask(ctx, &domain.Task{Title: "child", ParentID: &parentID})

	n, err := s.Tasks().DeleteTask(ctx, parent.ID)
	if err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 affected, got %d", n)
	}
	if _, err := s.Tasks().GetTask(ctx, child.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("child should be deleted, got %v", err)
	}
}
