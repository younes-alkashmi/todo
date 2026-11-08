package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/younes-alkashmi/todo/internal/domain"
	"github.com/younes-alkashmi/todo/internal/store"
)

var (
	ErrProjectNotFound = errors.New("project not found")
	ErrTaskNotFound    = errors.New("task not found")
	ErrParentNotFound  = errors.New("parent task not found")
	ErrForbidden       = errors.New("forbidden")
)

type TaskService struct {
	store   *store.Store
	ownerID string // "local" in M1
}

func NewTaskService(s *store.Store, ownerID string) *TaskService {
	if ownerID == "" {
		ownerID = "local"
	}
	return &TaskService{store: s, ownerID: ownerID}
}

// CreateTaskInput is the shape Wails will bind to. All fields optional
// except Title. ProjectID/ParentID as *string mirrors the domain.
type CreateTaskInput struct {
	Title       string
	Description string
	ProjectID   *string
	ParentID    *string
	DueDate     *string // "YYYY-MM-DD" or nil/empty
	Tags        []string
}

func (s *TaskService) CreateTask(ctx context.Context, in CreateTaskInput) (*domain.Task, error) {
	t := &domain.Task{
		OwnerUID:    s.ownerID,
		ProjectID:   in.ProjectID,
		ParentID:    in.ParentID,
		Title:       in.Title,
		Description: in.Description,
		Tags:        in.Tags,
	}
	t.NormalizeInput()
	if err := t.Validate(); err != nil {
		return nil, err
	}
	if err := s.checkDueDate(&t.DueDate, in.DueDate); err != nil {
		return nil, err
	}
	if err := s.validateParent(ctx, t); err != nil {
		return nil, err
	}
	if err := s.validateProject(ctx, t.ProjectID); err != nil {
		return nil, err
	}
	return s.store.Tasks().CreateTask(ctx, t)
}

func (s *TaskService) UpdateTask(ctx context.Context, id string, in CreateTaskInput) (*domain.Task, error) {
	cur, err := s.store.Tasks().GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	// Owner check — trivial in M1, real in M2.
	if cur.OwnerUID != s.ownerID {
		return nil, ErrForbidden
	}

	cur.Title = in.Title
	cur.Description = in.Description
	cur.Tags = in.Tags
	cur.NormalizeInput()
	if err := cur.Validate(); err != nil {
		return nil, err
	}
	if err := s.checkDueDate(&cur.DueDate, in.DueDate); err != nil {
		return nil, err
	}

	// Parent changes: validate against the *target* parent.
	if in.ParentID != nil || cur.ParentID != nil {
		probe := &domain.Task{ID: cur.ID, ProjectID: cur.ProjectID, ParentID: in.ParentID}
		if err := s.validateParent(ctx, probe); err != nil {
			return nil, err
		}
		cur.ParentID = in.ParentID
	}
	cur.ProjectID = in.ProjectID
	if err := s.validateProject(ctx, cur.ProjectID); err != nil {
		return nil, err
	}
	return s.store.Tasks().UpdateTask(ctx, cur)
}

func (s *TaskService) SetCompleted(ctx context.Context, id string, done bool) (*domain.Task, error) {
	cur, err := s.store.Tasks().GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	if cur.OwnerUID != s.ownerID {
		return nil, ErrForbidden
	}
	cur.Completed = done
	return s.store.Tasks().UpdateTask(ctx, cur)
}

func (s *TaskService) DeleteTask(ctx context.Context, id string) (int, error) {
	cur, err := s.store.Tasks().GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return 0, ErrTaskNotFound
		}
		return 0, err
	}
	if cur.OwnerUID != s.ownerID {
		return 0, ErrForbidden
	}
	return s.store.Tasks().DeleteTask(ctx, id)
}

func (s *TaskService) ListTasks(ctx context.Context, projectID *string) ([]*domain.Task, error) {
	return s.store.Tasks().ListTasks(ctx, projectID)
}

// CountChildren returns how many direct children a task has — used by
// the delete-confirm dialog (US-04).
func (s *TaskService) CountChildren(ctx context.Context, id string) (int, error) {
	tasks, err := s.store.Tasks().ListTasks(ctx, nil)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range tasks {
		if t.ParentID != nil && *t.ParentID == id {
			n++
		}
	}
	return n, nil
}

// ---- helpers ----

func (s *TaskService) checkDueDate(dst **domain.Date, raw *string) error {
	if raw == nil || *raw == "" {
		*dst = nil
		return nil
	}
	d, err := domain.ParseDate(*raw)
	if err != nil {
		return fmt.Errorf("due date: %w", err)
	}
	*dst = &d
	return nil
}

func (s *TaskService) validateParent(ctx context.Context, child *domain.Task) error {
	if child.ParentID == nil {
		return domain.ValidateHierarchy(nil, child)
	}
	parent, err := s.store.Tasks().GetTask(ctx, *child.ParentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrParentNotFound
		}
		return err
	}
	if parent.OwnerUID != s.ownerID {
		return ErrForbidden
	}
	return domain.ValidateHierarchy(parent, child)
}

func (s *TaskService) validateProject(ctx context.Context, projectID *string) error {
	if projectID == nil {
		return nil
	}
	projects, err := s.store.Projects().List(ctx)
	if err != nil {
		return err
	}
	for _, p := range projects {
		if p.ID == *projectID {
			if p.OwnerUID != s.ownerID {
				return ErrForbidden
			}
			return nil
		}
	}
	return ErrProjectNotFound
}
