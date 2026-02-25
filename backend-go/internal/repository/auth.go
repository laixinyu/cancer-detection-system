package repository

// 文件： internal/repository/auth.go
// 用途：仓储层，负责数据访问与持久化。

import (
	"context"

	"cancer-detection-backend/internal/domain"

	"gorm.io/gorm"
)

type AuthRepository interface {
	CountByEmail(ctx context.Context, email string) (int64, error)
	CreateUser(ctx context.Context, user *domain.User) error
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
}

type gormAuthRepository struct {
	db *gorm.DB
}

func NewGormAuthRepository(db *gorm.DB) AuthRepository {
	return &gormAuthRepository{db: db}
}

func (r *gormAuthRepository) CountByEmail(ctx context.Context, email string) (int64, error) {
	var cnt int64
	err := r.db.WithContext(ctx).Model(&domain.User{}).Where("email = ?", email).Count(&cnt).Error
	return cnt, err
}

func (r *gormAuthRepository) CreateUser(ctx context.Context, user *domain.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *gormAuthRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
