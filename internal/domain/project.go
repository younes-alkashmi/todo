package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const MaxProjectNameLen = 100

var (
	ErrProjectNameRequired = errors.New("project name is required")
	ErrProjectNameTooLong  = fmt.Errorf("project name must be at most %d characters", MaxProjectNameLen)
)

type Project struct {
	ID        string
	OwnerUID  string
	Name      string
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

func (p *Project) NormalizeInput() {
	p.Name = strings.TrimSpace(p.Name)
}

func (p *Project) Validate() error {
	if p.Name == "" {
		return ErrProjectNameRequired
	}
	if len([]rune(p.Name)) > MaxProjectNameLen {
		return ErrProjectNameTooLong
	}
	return nil
}
