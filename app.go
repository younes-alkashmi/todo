package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/younes-alkashmi/todo/internal/auth"
	"github.com/younes-alkashmi/todo/internal/domain"
	"github.com/younes-alkashmi/todo/internal/service"
	"github.com/younes-alkashmi/todo/internal/store"
	clientSync "github.com/younes-alkashmi/todo/internal/sync"
)

type App struct {
	ctx      context.Context
	tasks    *service.TaskService
	projects *service.ProjectService

	firebaseAPIKey string
	apiBaseURL     string // e.g. "http://localhost:8080"
	sessionToken   string
	currentUID     string
	store          *store.Store
	syncMu         sync.Mutex
	syncCancel     context.CancelFunc
}

func NewApp(
	tasks *service.TaskService,
	projects *service.ProjectService,
	store *store.Store,
	firebaseAPIKey string,
	apiBaseURL string,
) *App {
	return &App{
		ctx:            context.Background(),
		tasks:          tasks,
		projects:       projects,
		store:          store,
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

	if err := a.store.Meta().Set(a.ctx, "last_uid", resp.LocalID); err != nil {
		log.Printf("meta set failed: %v", err)
	}

	a.sessionToken = resp.IDToken
	a.currentUID = resp.LocalID
	a.startSyncLoop()
	return SignInResult{UID: resp.LocalID, Email: resp.Email}, nil
}

func (a *App) SignOut() error {
	if a.currentUID != "" {
		_ = auth.DeleteRefreshToken(a.currentUID)
		_ = a.store.Meta().Set(a.ctx, "last_uid", "")
	}
	a.sessionToken = ""
	a.currentUID = ""
	a.stopSyncLoop()
	return nil
}

func (a *App) RestoreSession() (*SignInResult, error) {
	// We don't know the UID without it — try each entry? No: store UID in a
	// small file next to the DB. Simpler: keep it in sqlite meta.
	uid, err := a.store.Meta().Get("last_uid")
	if err != nil || uid == "" {
		return nil, nil // no prior session
	}
	refreshToken, err := auth.LoadRefreshToken(uid)
	if err != nil {
		return nil, nil // keyring gone; user must sign in again
	}
	idToken, err := auth.RefreshIDToken(a.ctx, a.firebaseAPIKey, refreshToken)
	if err != nil {
		return nil, err
	}
	a.sessionToken = idToken
	a.currentUID = uid
	a.startSyncLoop()
	return &SignInResult{UID: uid}, nil
}

func (a *App) CurrentUser() *SignInResult {
	if a.currentUID == "" {
		return nil
	}
	return &SignInResult{UID: a.currentUID}
}

var ErrNotSignedIn = errors.New("not signed in")

func (a *App) SessionInfo() *SignInResult {
	if a.currentUID == "" {
		return nil
	}
	return &SignInResult{UID: a.currentUID}
}

func (a *App) startSyncLoop() {
	a.stopSyncLoop()
	ctx, cancel := context.WithCancel(a.ctx)
	a.syncCancel = cancel
	log.Printf("sync: starting loop, token length=%d, apiBase=%s", len(a.sessionToken), a.apiBaseURL)

	client := clientSync.New(a.apiBaseURL, a.sessionToken, a.store)
	go func() {
		log.Println("sync: loop started")
		if err := client.SyncOnce(ctx); err != nil {
			log.Printf("sync: initial: %v", err)
		}
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("sync: loop stopped")
				return
			case <-ticker.C:
				if err := client.SyncOnce(ctx); err != nil {
					log.Printf("sync: %v", err)
				} else {
					log.Println("sync: ok")
				}
			}
		}
	}()
}

func (a *App) stopSyncLoop() {
	a.syncMu.Lock()
	defer a.syncMu.Unlock()
	if a.syncCancel != nil {
		a.syncCancel()
		a.syncCancel = nil
	}
}

func (a *App) SyncStatus() string {
	pending, _ := a.store.Tasks().PendingMutations(a.ctx)
	if len(pending) == 0 {
		if a.sessionToken == "" {
			return "Offline · Local only"
		}
		return "Synced"
	}

	for _, m := range pending {
		if m.State == "conflict" {
			return "Conflict"
		}
	}
	return "Pending"
}
