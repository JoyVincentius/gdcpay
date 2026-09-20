package domain

import (
	"context"
	"time"
)

type Team struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

type TeamRepository interface {
	Create(ctx context.Context, team *Team) error
	FindByName(ctx context.Context, name string) (*Team, error)
	FindByID(ctx context.Context, id string) (*Team, error)
}
