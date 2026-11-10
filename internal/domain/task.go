package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	MaxTitleLen       = 200
	MaxDescriptionLen = 10_000
	MaxTags           = 20
	MaxTagLen         = 30
)

var (
	ErrTitleRequired = errors.New("title is required")
	ErrTitleTooLong  = fmt.Errorf("title must be at most %d characters", MaxTitleLen)
	ErrDescTooLong   = fmt.Errorf("description must be at most %d characters", MaxDescriptionLen)
	ErrTooManyTags   = fmt.Errorf("at most %d tags are allowed", MaxTags)
	ErrTagTooLong    = fmt.Errorf("each tag must be at most %d characters", MaxTagLen)
	ErrParentNotRoot = errors.New("parent must be a root task")
	ErrSecondLevel   = errors.New("subtasks cannot have subtasks")
	ErrSelfParent    = errors.New("a task cannot be its own parent")
	ErrCycle         = errors.New("task hierarchy would contain a cycle")
	ErrScopeMismatch = errors.New("subtask must inherit parent scope")
)

type Date struct {
	Year  int
	Month time.Month
	Day   int
}

func NewDate(y int, m time.Month, d int) Date { return Date{y, m, d} }

func (d Date) IsZero() bool { return d == Date{} }

func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

func ParseDate(s string) (Date, error) {
	if s == "" {
		return Date{}, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return Date{}, fmt.Errorf("invalid date %q: must be YYYY-MM-DD", s)
	}
	return Date{t.Year(), t.Month(), t.Day()}, nil
}

type Task struct {
	ID          string     `json:"id"`
	OwnerUID    string     `json:"ownerUid"`
	ProjectID   *string    `json:"projectId"`
	ParentID    *string    `json:"parentId"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueDate     *Date      `json:"dueDate,omitempty"`
	Tags        []string   `json:"tags"`
	Completed   bool       `json:"completed"`
	Version     int        `json:"version"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	DeletedAt   *time.Time `json:"deletedAt,omitempty"`
}

func (t *Task) NormalizeInput() {
	t.Title = strings.TrimSpace(t.Title)
	t.Description = strings.TrimSpace(t.Description)

	seen := make(map[string]struct{}, len(t.Tags))
	out := make([]string, 0, len(t.Tags))
	for _, tag := range t.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}
	t.Tags = out
}

func (t *Task) Validate() error {
	if t.Title == "" {
		return ErrTitleRequired
	}
	// Count runes, not bytes, so non-ASCII titles aren't penalized.
	if len([]rune(t.Title)) > MaxTitleLen {
		return ErrTitleTooLong
	}
	if len([]rune(t.Description)) > MaxDescriptionLen {
		return ErrDescTooLong
	}
	if len(t.Tags) > MaxTags {
		return ErrTooManyTags
	}
	for _, tag := range t.Tags {
		if len([]rune(tag)) > MaxTagLen {
			return ErrTagTooLong
		}
	}
	return nil
}

func ValidateHierarchy(parent, child *Task) error {
	if parent == nil {
		return nil
	}
	if child != nil && parent.ID == child.ID {
		return ErrSelfParent
	}
	if parent.ParentID != nil {
		return ErrSecondLevel
	}
	if child != nil && !sameScope(parent, child) {
		return ErrScopeMismatch
	}
	return nil
}

func sameScope(a, b *Task) bool {
	ap, bp := a.ProjectID, b.ProjectID
	if ap == nil || bp == nil {
		return ap == nil && bp == nil
	}
	return *ap == *bp
}

func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + d.String() + `"`), nil
}

func (d *Date) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	s := strings.Trim(string(b), `"`)
	if s == "" {
		return nil
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
