package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"gdcpay/internal/domain"
	"gdcpay/internal/dto"
	"gdcpay/internal/pkg/jwtutil"
)

type AuthService struct {
	users     domain.UserRepository
	teams     domain.TeamRepository
	tokenizer *jwtutil.Tokenizer
	tokenTTL  time.Duration
}

func NewAuthService(users domain.UserRepository, teams domain.TeamRepository, tokenizer *jwtutil.Tokenizer, tokenTTL time.Duration) *AuthService {
	return &AuthService{users: users, teams: teams, tokenizer: tokenizer, tokenTTL: tokenTTL}
}

func (s *AuthService) Register(ctx context.Context, req dto.RegisterRequest) (*dto.AuthResponse, error) {
	email := strings.ToLower(req.Email)

	_, err := s.users.FindByEmail(ctx, email)
	if err == nil {
		return nil, domain.ErrEmailAlreadyExists
	}
	if !errors.Is(err, domain.ErrUserNotFound) {
		return nil, domain.ErrInternal
	}

	team, err := s.resolveTeam(ctx, req.Name, req.TeamName)
	if err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, domain.ErrInternal
	}

	user := &domain.User{
		ID:           uuid.NewString(),
		TeamID:       team.ID,
		Name:         req.Name,
		Email:        email,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, domain.ErrInternal
	}

	return s.issueToken(user)
}

func (s *AuthService) Login(ctx context.Context, req dto.LoginRequest) (*dto.AuthResponse, error) {
	user, err := s.users.FindByEmail(ctx, strings.ToLower(req.Email))
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, domain.ErrInternal
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	return s.issueToken(user)
}

// resolveTeam joins an existing team by name, or creates a new one. This
// keeps registration self-service (no separate team-management endpoints)
// while still letting two users share a team by registering with the same
// team_name, which the task-assignment feature relies on.
func (s *AuthService) resolveTeam(ctx context.Context, userName, teamName string) (*domain.Team, error) {
	name := strings.TrimSpace(teamName)
	if name == "" {
		name = userName + "'s team"
	}

	team, err := s.teams.FindByName(ctx, name)
	if err == nil {
		return team, nil
	}
	if !errors.Is(err, domain.ErrTeamNotFound) {
		return nil, domain.ErrInternal
	}

	team = &domain.Team{
		ID:        uuid.NewString(),
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.teams.Create(ctx, team); err != nil {
		return nil, domain.ErrInternal
	}
	return team, nil
}

func (s *AuthService) issueToken(user *domain.User) (*dto.AuthResponse, error) {
	token, err := s.tokenizer.Generate(user.ID, user.TeamID)
	if err != nil {
		return nil, domain.ErrInternal
	}

	return &dto.AuthResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(s.tokenTTL.Seconds()),
		User: dto.UserResponse{
			ID:     user.ID,
			Name:   user.Name,
			Email:  user.Email,
			TeamID: user.TeamID,
		},
	}, nil
}
