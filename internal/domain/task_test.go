package domain

import (
	"errors"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func TestTaskValidate(t *testing.T) {
	tooLongTitle := strings.Repeat("a", MaxTitleLen+1)
	tooLongDesc := strings.Repeat("x", MaxDescriptionLen+1)
	tooLongTag := strings.Repeat("t", MaxTagLen+1)

	manyTags := make([]string, MaxTags+1)
	for i := range manyTags {
		manyTags[i] = string(rune('a' + i%26))
	}

	cases := []struct {
		name    string
		mutate  func(*Task)
		wantErr error
	}{
		{
			name:   "valid minimal",
			mutate: func(*Task) {},
		},
		{
			name:    "empty title",
			mutate:  func(t *Task) { t.Title = "" },
			wantErr: ErrTitleRequired,
		},
		{
			name:    "whitespace title",
			mutate:  func(t *Task) { t.Title = "   "; t.NormalizeInput() },
			wantErr: ErrTitleRequired,
		},
		{
			name:    "title at limit",
			mutate:  func(t *Task) { t.Title = strings.Repeat("a", MaxTitleLen) },
			wantErr: nil,
		},
		{
			name:    "title over limit",
			mutate:  func(t *Task) { t.Title = tooLongTitle },
			wantErr: ErrTitleTooLong,
		},
		{
			name:    "description over limit",
			mutate:  func(t *Task) { t.Description = tooLongDesc },
			wantErr: ErrDescTooLong,
		},
		{
			name:    "too many tags",
			mutate:  func(t *Task) { t.Tags = manyTags },
			wantErr: ErrTooManyTags,
		},
		{
			name:    "tag over limit",
			mutate:  func(t *Task) { t.Tags = []string{tooLongTag} },
			wantErr: ErrTagTooLong,
		},
		{
			name:   "unicode title at limit",
			mutate: func(t *Task) { t.Title = strings.Repeat("é", MaxTitleLen) },
			// wantErr nil: 200 runes, not 200 bytes
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := &Task{Title: "ok"}
			tc.mutate(task)
			err := task.Validate()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestTaskNormalizeInput(t *testing.T) {
	task := &Task{
		Title:       "  Buy milk  ",
		Description: "\n  note  ",
		Tags:        []string{" Go ", "go", "SQL", "  ", "sql", "Wails"},
	}
	task.NormalizeInput()

	if task.Title != "Buy milk" {
		t.Errorf("Title = %q", task.Title)
	}
	if task.Description != "note" {
		t.Errorf("Description = %q", task.Description)
	}
	// Deduped case-insensitively; first casing wins; order preserved.
	wantTags := []string{"Go", "SQL", "Wails"}
	if len(task.Tags) != len(wantTags) {
		t.Fatalf("Tags = %v, want %v", task.Tags, wantTags)
	}
	for i, w := range wantTags {
		if task.Tags[i] != w {
			t.Errorf("Tags[%d] = %q, want %q", i, task.Tags[i], w)
		}
	}
}

func TestValidateHierarchy(t *testing.T) {
	proj := "p1"
	root := &Task{ID: "root", Title: "root"}
	child := &Task{ID: "child", Title: "child", ParentID: ptr("root")}
	projRoot := &Task{ID: "proot", ProjectID: &proj, Title: "proot"}

	cases := []struct {
		name    string
		parent  *Task
		child   *Task
		wantErr error
	}{
		{"no parent is fine", nil, child, nil},
		{"root parent ok", root, child, nil},
		{"self parent", root, root, ErrSelfParent},
		{"parent has parent", child, &Task{ID: "x", ParentID: ptr("child")}, ErrSecondLevel},
		{"scope mismatch", root, projRoot, ErrScopeMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateHierarchy(tc.parent, tc.child); !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestDateRoundTrip(t *testing.T) {
	cases := []string{"", "2026-11-10", "2026-02-28", "2024-02-29"}
	for _, s := range cases {
		d, err := ParseDate(s)
		if err != nil {
			t.Fatalf("ParseDate(%q): %v", s, err)
		}
		if got := d.String(); got != s {
			t.Errorf("roundtrip %q -> %q", s, got)
		}
	}
	if _, err := ParseDate("2026-11-10T00:00:00Z"); err == nil {
		t.Error("expected error for RFC3339 date")
	}
	if _, err := ParseDate("2026/11/10"); err == nil {
		t.Error("expected error for slash-separated date")
	}
}
