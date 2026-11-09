package main

import (
	"context"
	"log"
	"time"

	"github.com/younes-alkashmi/todo/internal/auth"
	"github.com/younes-alkashmi/todo/internal/domain"
	"github.com/younes-alkashmi/todo/internal/service"
)

type App struct {
	ctx      context.Context
	tasks    *service.TaskService
	projects *service.ProjectService

	firebaseAPIKey string
	apiBaseURL     string // e.g. "http://localhost:8080"
	sessionToken   string
	currentUID     string
}

func NewApp(
	tasks *service.TaskService,
	projects *service.ProjectService,
	firebaseAPIKey string,
	apiBaseURL string,
) *App {
	return &App{
		ctx:            context.Background(),
		tasks:          tasks,
		projects:       projects,
		firebaseAPIKey: firebaseAPIKey,
		apiBaseURL:     apiBaseURL,
	}
}

type TaskDTO struct {
	ID          string   `json:"id"`
	OwnerUID    string   `json:"ownerUid"`
	ProjectID   *string  `json:"projectId"`
	ParentID    *string  `json:"parentId"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	DueDate     *string  `json:"dueDate"`
	Tags        []string `json:"tags"`
	Completed   bool     `json:"completed"`
	Version     int      `json:"version"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

func toTaskDTO(t *domain.Task) TaskDTO {
	dto := TaskDTO{
		ID:          t.ID,
		OwnerUID:    t.OwnerUID,
		ProjectID:   t.ProjectID,
		ParentID:    t.ParentID,
		Title:       t.Title,
		Description: t.Description,
		Tags:        t.Tags,
		Completed:   t.Completed,
		Version:     t.Version,
		CreatedAt:   t.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:   t.UpdatedAt.Format(time.RFC3339Nano),
	}
	if t.DueDate != nil {
		s := t.DueDate.String()
		dto.DueDate = &s
	}
	if dto.Tags == nil {
		dto.Tags = []string{}
	}
	return dto
}

type CreateTaskIn struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	ProjectID   *string  `json:"projectId"`
	ParentID    *string  `json:"parentId"`
	DueDate     *string  `json:"dueDate"`
	Tags        []string `json:"tags"`
}

type ProjectDTO struct {
	ID        string `json:"id"`
	OwnerUID  string `json:"ownerUid"`
	Name      string `json:"name"`
	Version   int    `json:"version"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type SignInResult struct {
	UID   string `json:"uid"`
	Email string `json:"email"`
}

func toProjectDTO(p *domain.Project) ProjectDTO {
	return ProjectDTO{
		ID:        p.ID,
		OwnerUID:  p.OwnerUID,
		Name:      p.Name,
		Version:   p.Version,
		CreatedAt: p.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt: p.UpdatedAt.Format(time.RFC3339Nano),
	}
}

func (a *App) CreateTask(in CreateTaskIn) (TaskDTO, error) {
	t, err := a.tasks.CreateTask(a.ctx, service.CreateTaskInput{
		Title:       in.Title,
		Description: in.Description,
		ProjectID:   in.ProjectID,
		ParentID:    in.ParentID,
		DueDate:     in.DueDate,
		Tags:        in.Tags,
	})
	if err != nil {
		return TaskDTO{}, err
	}
	return toTaskDTO(t), nil
}

func (a *App) UpdateTask(id string, in CreateTaskIn) (TaskDTO, error) {
	t, err := a.tasks.UpdateTask(a.ctx, id, service.CreateTaskInput{
		Title:       in.Title,
		Description: in.Description,
		ProjectID:   in.ProjectID,
		ParentID:    in.ParentID,
		DueDate:     in.DueDate,
		Tags:        in.Tags,
	})
	if err != nil {
		return TaskDTO{}, err
	}
	return toTaskDTO(t), nil
}

func (a *App) SetTaskCompleted(id string, done bool) (TaskDTO, error) {
	t, err := a.tasks.SetCompleted(a.ctx, id, done)
	if err != nil {
		return TaskDTO{}, err
	}
	return toTaskDTO(t), nil
}

func (a *App) DeleteTask(id string) (int, error) {
	return a.tasks.DeleteTask(a.ctx, id)
}

func (a *App) ListTasks(projectID *string) ([]TaskDTO, error) {
	tasks, err := a.tasks.ListTasks(a.ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]TaskDTO, len(tasks))
	for i, t := range tasks {
		out[i] = toTaskDTO(t)
	}
	return out, nil
}

func (a *App) CreateProject(name string) (ProjectDTO, error) {
	p, err := a.projects.Create(a.ctx, service.CreateProjectInput{Name: name})
	if err != nil {
		return ProjectDTO{}, err
	}
	return toProjectDTO(p), nil
}

func (a *App) ListProjects() ([]ProjectDTO, error) {
	projects, err := a.projects.List(a.ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectDTO, len(projects))
	for i, p := range projects {
		out[i] = toProjectDTO(p)
	}
	return out, nil
}

func (a *App) DeleteProject(id string) (int, error) {
	return a.projects.Delete(a.ctx, id)
}

func (a *App) Ping() string { return "pong" }

func (a *App) SignIn(email, password string) (SignInResult, error) {
	resp, err := auth.SignInEmailPassword(a.ctx, a.firebaseAPIKey, email, password)
	if err != nil {
		return SignInResult{}, err
	}

	// Persist refresh token to OS keyring — this is the M3 secure-storage requirement
	if err := auth.StoreRefreshToken(resp.LocalID, resp.RefreshToken); err != nil {
		log.Printf("keyring store failed: %v", err)
	}

	a.sessionToken = resp.IDToken
	a.currentUID = resp.LocalID
	return SignInResult{UID: resp.LocalID, Email: resp.Email}, nil
}

func (a *App) SignOut() error {
	if a.currentUID != "" {
		_ = auth.DeleteRefreshToken(a.currentUID)
	}
	a.sessionToken = ""
	a.currentUID = ""
	return nil
}

func (a *App) CurrentUser() *SignInResult {
	if a.currentUID == "" {
		return nil
	}
	return &SignInResult{UID: a.currentUID}
}
