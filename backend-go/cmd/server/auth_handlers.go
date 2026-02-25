package main

// 文件： cmd/server/auth_handlers.go
// 用途：网关的处理器、中间件与对外 HTTP API 路由装配。

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"cancer-detection-backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func (a *app) register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || req.Email == "" || len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name/email/password is invalid"})
		return
	}

	role := strings.ToUpper(strings.TrimSpace(req.Role))
	if role == "" {
		role = "PATIENT"
	}
	if role != "PATIENT" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only PATIENT role can self-register"})
		return
	}

	user, err := a.authService.Register(c.Request.Context(), service.RegisterInput{
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
		Role:     req.Role,
		Phone:    req.Phone,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidRegisterPayload):
			c.JSON(http.StatusBadRequest, gin.H{"error": "name/email/password is invalid"})
		case errors.Is(err, service.ErrOnlyPatientSelfRegister):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Only PATIENT role can self-register"})
		case errors.Is(err, service.ErrUserExists):
			c.JSON(http.StatusBadRequest, gin.H{"error": "User already exists"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		}
		return
	}
	out := userDTO{ID: user.ID, Email: user.Email, Name: user.Name, Role: user.Role}
	c.JSON(http.StatusCreated, gin.H{"user": out})
}

func (a *app) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	user, err := a.authService.Login(c.Request.Context(), service.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query user"})
		return
	}
	userDTO := userDTO{ID: user.ID, Email: user.Email, Name: user.Name, Role: user.Role}

	now := time.Now().UTC()
	exp := now.Add(24 * time.Hour)
	claims := authClaims{
		UserID: userDTO.ID,
		Role:   userDTO.Role,
		Email:  userDTO.Email,
		Name:   userDTO.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userDTO.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err := jwtToken.SignedString(a.jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":     token,
		"user":      userDTO,
		"expiresAt": exp.Format(time.RFC3339),
	})
}

func toStrPtr(v string) *string {
	s := strings.TrimSpace(v)
	if s == "" {
		return nil
	}
	return &s
}

func (a *app) authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing bearer token"})
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
		claims := &authClaims{}
		parsed, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return a.jwtSecret, nil
		})
		if err != nil || !parsed.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}

		c.Set("claims", claims)
		c.Next()
	}
}
