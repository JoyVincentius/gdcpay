package postgres

import (
	"context"
	"database/sql"
	"errors"

	"gdcpay/internal/domain"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) q(ctx context.Context) querier {
	if tx := txFromContext(ctx); tx != nil {
		return tx
	}
	return r.db
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	const query = `
		INSERT INTO users (id, team_id, name, email, password_hash, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.q(ctx).ExecContext(ctx, query,
		user.ID, user.TeamID, user.Name, user.Email, user.PasswordHash, user.CreatedAt,
	)
	return err
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	const query = `
		SELECT id, team_id, name, email, password_hash, created_at
		FROM users
		WHERE email = $1
	`
	return r.scanUser(r.q(ctx).QueryRowContext(ctx, query, email))
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	const query = `
		SELECT id, team_id, name, email, password_hash, created_at
		FROM users
		WHERE id = $1
	`
	return r.scanUser(r.q(ctx).QueryRowContext(ctx, query, id))
}

func (r *UserRepository) scanUser(row *sql.Row) (*domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.TeamID, &u.Name, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}
