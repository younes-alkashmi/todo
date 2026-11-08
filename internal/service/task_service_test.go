package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/younes-alkashmi/todo/internal/domain"
	"github.com/younes-alkashmi/todo/internal/store"
)

func newServices(t *testing.T) (*TaskService, *ProjectService, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return NewTaskService(s, "local"), NewProjectService(s, "local"), s
}

func TestCreateTaskRejectsBlankTitle(t *testing.T) {
	tasks, _, _ := newServices(t)
	_, err := tasks.CreateTask(context.Background(), CreateTaskInput{Title: "   "})
	if !errors.Is(err, domain.ErrTitleRequired) {
		t.Fatalf("got %v, want ErrTitleRequired", err)
	}
}

func TestCreateTaskWithDueDateRoundTrip(t *testing.T) {
	tasks, _, _ := newServices(t)
	due := "2026-11-10"
	got, err := tasks.CreateTask(context.Background(), CreateTaskInput{
		Title:   "ship it",
		DueDate: &due,
		Tags:    []string{" go ", "GO", "sqlite"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.DueDate == nil || got.DueDate.String() != "2026-11-10" {
		t.Fatalf("due date = %v", got.DueDate)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "go" || got.Tags[1] != "sqlite" {
		t.Fatalf("tags = %v", got.Tags)
	}
}

func TestCreateSubtaskUnderProject(t *testing.T) {
	tasks, projects, _ := newServices(t)
	ctx := context.Background()
	p, err := projects.Create(ctx, CreateProjectInput{Name: "Learn Wails"})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	parent, err := tasks.CreateTask(ctx, CreateTaskInput{Title: "root", ProjectID: &p.ID})
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	child, err := tasks.CreateTask(ctx, CreateTaskInput{
		Title:     "child",
		ProjectID: &p.ID,
		ParentID:  &parent.ID,
	})
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Fatalf("child parent = %v", child.ParentID)
	}
}

func TestCreateSecondLevelSubtaskRejected(t *testing.T) {
	tasks, _, _ := newServices(t)
	ctx := context.Background()
	parent, _ := tasks.CreateTask(ctx, CreateTaskInput{Title: "root"})
	child, _ := tasks.CreateTask(ctx, CreateTaskInput{Title: "child", ParentID: &parent.ID})
	_, err := tasks.CreateTask(ctx, CreateTaskInput{Title: "grandchild", ParentID: &child.ID})
	if !errors.Is(err, domain.ErrSecondLevel) {
		t.Fatalf("got %v, want ErrSecondLevel", err)
	}
}

func TestSubtaskScopeMismatchRejected(t *testing.T) {
	tasks, projects, _ := newServices(t)
	ctx := context.Background()
	p, _ := projects.Create(ctx, CreateProjectInput{Name: "P"})
	projRoot, _ := tasks.CreateTask(ctx, CreateTaskInput{Title: "root", ProjectID: &p.ID})
	globalRoot, _ := tasks.CreateTask(ctx, CreateTaskInput{Title: "global"})

	// Attach the global root as a child of the project root: scope mismatch.
	_, err := tasks.CreateTask(ctx, CreateTaskInput{
		Title:     "mixed",
		ProjectID: nil,          // global
		ParentID:  &projRoot.ID, // project-scoped parent
	})
	_ = globalRoot
	if !errors.Is(err, domain.ErrScopeMismatch) {
		t.Fatalf("got %v, want ErrScopeMismatch", err)
	}
}

func TestSetCompletedIndependentOfParent(t *testing.T) {
	tasks, _, _ := newServices(t)
	ctx := context.Background()
	parent, _ := tasks.CreateTask(ctx, CreateTaskInput{Title: "root"})
	child, _ := tasks.CreateTask(ctx, CreateTaskInput{Title: "child", ParentID: &parent.ID})

	// Completing the parent must not change the child.
	if _, err := tasks.SetCompleted(ctx, parent.ID, true); err != nil {
		t.Fatalf("complete parent: %v", err)
	}
	reloaded, err := tasks.ListTasks(ctx, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, task := range reloaded {
		if task.ID == child.ID && task.Completed {
			t.Fatal("child completion should be independent of parent")
		}
	}
}

func TestDeleteTaskCascadesChildren(t *testing.T) {
	tasks, _, s := newServices(t)
	ctx := context.Background()
	parent, _ := tasks.CreateTask(ctx, CreateTaskInput{Title: "root"})
	child, _ := tasks.CreateTask(ctx, CreateTaskInput{Title: "child", ParentID: &parent.ID})

	n, err := tasks.DeleteTask(ctx, parent.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 2 {
		t.Fatalf("affected = %d, want 2", n)
	}
	if _, err := s.Tasks().GetTask(ctx, child.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("child should be gone, got %v", err)
	}
}

func TestDeleteProjectCountsTasks(t *testing.T) {
	tasks, projects, _ := newServices(t)
	ctx := context.Background()
	p, _ := projects.Create(ctx, CreateProjectInput{Name: "P"})
	_, _ = tasks.CreateTask(ctx, CreateTaskInput{Title: "a", ProjectID: &p.ID})
	_, _ = tasks.CreateTask(ctx, CreateTaskInput{Title: "b", ProjectID: &p.ID})

	n, err := projects.Delete(ctx, p.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 2 {
		t.Fatalf("affected tasks = %d, want 2", n)
	}
}

func TestCreateTaskInMissingProjectRejected(t *testing.T) {
	tasks, _, _ := newServices(t)
	missing := "does-not-exist"
	_, err := tasks.CreateTask(context.Background(), CreateTaskInput{Title: "x", ProjectID: &missing})
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("got %v, want ErrProjectNotFound", err)
	}
}
