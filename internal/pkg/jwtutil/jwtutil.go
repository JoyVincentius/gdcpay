package jwtutil

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid or expired token")

type Claims struct {
	UserID string `json:"user_id"`
	TeamID string `json:"team_id"`
	jwt.RegisteredClaims
}

type Tokenizer struct {
	secret   []byte
	expireIn time.Duration
}

func NewTokenizer(secret string, expireIn time.Duration) *Tokenizer {
	return &Tokenizer{secret: []byte(secret), expireIn: expireIn}
}

func (t *Tokenizer) Generate(userID, teamID string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		TeamID: teamID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.expireIn)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(t.secret)
}

func (t *Tokenizer) Verify(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return t.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
