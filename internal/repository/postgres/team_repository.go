package postgres

import (
	"context"
	"database/sql"
	"errors"

	"gdcpay/internal/domain"
)

type TeamRepository struct {
	db *sql.DB
}

func NewTeamRepository(db *sql.DB) *TeamRepository {
	return &TeamRepository{db: db}
}

func (r *TeamRepository) Create(ctx context.Context, team *domain.Team) error {
	const query = `
		INSERT INTO teams (id, name, created_at)
		VALUES ($1, $2, $3)
	`
	_, err := r.db.ExecContext(ctx, query, team.ID, team.Name, team.CreatedAt)
	return err
}

func (r *TeamRepository) FindByName(ctx context.Context, name string) (*domain.Team, error) {
	const query = `
		SELECT id, name, created_at
		FROM teams
		WHERE name = $1
	`
	return r.scanTeam(r.db.QueryRowContext(ctx, query, name))
}

func (r *TeamRepository) FindByID(ctx context.Context, id string) (*domain.Team, error) {
	const query = `
		SELECT id, name, created_at
		FROM teams
		WHERE id = $1
	`
	return r.scanTeam(r.db.QueryRowContext(ctx, query, id))
}

func (r *TeamRepository) scanTeam(row *sql.Row) (*domain.Team, error) {
	var t domain.Team
	err := row.Scan(&t.ID, &t.Name, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrTeamNotFound
		}
		return nil, err
	}
	return &t, nil
}
