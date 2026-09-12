package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/thvnhtai/gearshare/internal/auth"
)

var ErrInvalidCredentials = errors.New("user: invalid credentials")

type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, hash string) bool
}

type Service struct {
	repo    *Repository
	hasher  PasswordHasher
	issuer  *auth.JWTIssuer
}

func NewService(repo *Repository, hasher PasswordHasher, issuer *auth.JWTIssuer) *Service {
	return &Service{repo: repo, hasher: hasher, issuer: issuer}
}

type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
	Role        Role
}

type AuthResult struct {
	User        *User
	AccessToken string
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*AuthResult, error) {
	if in.Role == "" {
		in.Role = RoleRenter
	}

	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, fmt.Errorf("user: hash password: %w", err)
	}

	u := &User{
		Email:        in.Email,
		PasswordHash: hash,
		PasswordAlgo: PasswordAlgoBcrypt,
		DisplayName:  in.DisplayName,
		Role:         in.Role,
	}
	id, err := s.repo.Create(ctx, u)
	if err != nil {
		return nil, err
	}
	u.ID = id

	token, err := s.issuer.IssueAccessToken(u.ID, string(u.Role))
	if err != nil {
		return nil, fmt.Errorf("user: issue token: %w", err)
	}
	return &AuthResult{User: u, AccessToken: token}, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	u, err := s.repo.GetByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if !s.hasher.Verify(password, u.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	token, err := s.issuer.IssueAccessToken(u.ID, string(u.Role))
	if err != nil {
		return nil, fmt.Errorf("user: issue token: %w", err)
	}
	return &AuthResult{User: u, AccessToken: token}, nil
}

func (s *Service) GetByID(ctx context.Context, id int64) (*User, error) {
	return s.repo.GetByID(ctx, id)
}
