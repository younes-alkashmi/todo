package service

import (
	"context"
	// "errors"

	"github.com/younes-alkashmi/todo/internal/domain"
	"github.com/younes-alkashmi/todo/internal/store"
)

type ProjectService struct {
	store   *store.Store
	ownerID string
}

func NewProjectService(s *store.Store, ownerID string) *ProjectService {
	if ownerID == "" {
		ownerID = "local"
	}
	return &ProjectService{store: s, ownerID: ownerID}
}

type CreateProjectInput struct {
	Name string
}

func (s *ProjectService) Create(ctx context.Context, in CreateProjectInput) (*domain.Project, error) {
	p := &domain.Project{OwnerUID: s.ownerID, Name: in.Name}
	p.NormalizeInput()
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return s.store.Projects().Create(ctx, p)
}

func (s *ProjectService) List(ctx context.Context) ([]*domain.Project, error) {
	return s.store.Projects().List(ctx)
}

// Delete returns how many tasks were removed alongside the project.
// US-04: the UI must confirm with this count.
func (s *ProjectService) Delete(ctx context.Context, id string) (int, error) {
	projects, err := s.store.Projects().List(ctx)
	if err != nil {
		return 0, err
	}
	var found *domain.Project
	for _, p := range projects {
		if p.ID == id {
			found = p
			break
		}
	}
	if found == nil {
		return 0, ErrProjectNotFound
	}
	if found.OwnerUID != s.ownerID {
		return 0, ErrForbidden
	}
	return s.store.Projects().Delete(ctx, id)
}
