package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	jwt.RegisteredClaims
}

func authRequired(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing bearer token"})
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
		cl := &claims{}
		parsed, err := jwt.ParseWithClaims(token, cl, func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return secret, nil
		})
		if err != nil || !parsed.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}
		c.Set("claims", cl)
		c.Next()
	}
}

func mustClaims(c *gin.Context) *claims {
	raw, _ := c.Get("claims")
	out, _ := raw.(*claims)
	return out
}
