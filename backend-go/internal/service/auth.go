package service

import (
	"context"
	"errors"
	"strings"

	"cancer-detection-backend/internal/domain"
	"cancer-detection-backend/internal/repository"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthService struct {
	repo repository.AuthRepository
}

func NewAuthService(repo repository.AuthRepository) *AuthService {
	return &AuthService{repo: repo}
}

var (
	ErrInvalidRegisterPayload = errors.New("name/email/password is invalid")
	ErrOnlyPatientSelfRegister = errors.New("only patient role can self-register")
	ErrUserExists = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

type RegisterInput struct {
	Name     string
	Email    string
	Password string
	Role     string
	Phone    string
}

type AuthUser struct {
	ID    string
	Email string
	Name  string
	Role  string
}

type LoginInput struct {
	Email    string
	Password string
}

func (s *AuthService) Register(ctx context.Context, in RegisterInput) (*AuthUser, error) {
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || in.Email == "" || len(in.Password) < 8 {
		return nil, ErrInvalidRegisterPayload
	}

	role := strings.ToUpper(strings.TrimSpace(in.Role))
	if role == "" {
		role = "PATIENT"
	}
	if role != "PATIENT" {
		return nil, ErrOnlyPatientSelfRegister
	}

	exists, err := s.repo.CountByEmail(ctx, in.Email)
	if err != nil {
		return nil, err
	}
	if exists > 0 {
		return nil, ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), 12)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		Email:        in.Email,
		PasswordHash: string(hash),
		Role:         role,
		Name:         in.Name,
		Phone:        toStrPtr(in.Phone),
	}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	return &AuthUser{ID: user.ID, Email: user.Email, Name: user.Name, Role: user.Role}, nil
}

func (s *AuthService) Login(ctx context.Context, in LoginInput) (*AuthUser, error) {
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	user, err := s.repo.FindByEmail(ctx, in.Email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return &AuthUser{ID: user.ID, Email: user.Email, Name: user.Name, Role: user.Role}, nil
}

func toStrPtr(v string) *string {
	s := strings.TrimSpace(v)
	if s == "" {
		return nil
	}
	return &s
}

