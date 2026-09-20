package domain

import (
	"context"
	"time"
)

type User struct {
	ID           string
	TeamID       string
	Name         string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

// UserRepository is implemented by the persistence layer and consumed by
// services through this interface so business logic can be unit-tested with
// an in-memory fake instead of a live database.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id string) (*User, error)
}
