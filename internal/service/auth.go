// Package service holds use cases. It orchestrates domain types and repository
// ports and never runs SQL itself.
package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"local-tracker/internal/domain"
)

const (
	pinMinLength = 4
	pinMaxLength = 12
	tokenBytes   = 32
)

// AuthService implements first-run setup, PIN login, session lookup and logout.
type AuthService struct {
	users    domain.UserRepository
	sessions domain.SessionRepository
	ttl      time.Duration
	now      func() time.Time
}

// NewAuth wires the auth use cases. A non-positive TTL falls back to 30 days.
func NewAuth(users domain.UserRepository, sessions domain.SessionRepository, ttl time.Duration) *AuthService {
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	return &AuthService{users: users, sessions: sessions, ttl: ttl, now: time.Now}
}

// SessionTTL reports the configured session lifetime.
func (s *AuthService) SessionTTL() time.Duration { return s.ttl }

// SetupInput carries the two profiles entered in the wizard.
type SetupInput struct {
	Name1, Pin1 string
	Name2, Pin2 string
}

// NeedsSetup reports whether the first-run wizard is still open.
func (s *AuthService) NeedsSetup(ctx context.Context) (bool, error) {
	n, err := s.users.Count(ctx, domain.SystemActor())
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// Setup validates and creates exactly the two profiles, atomically.
func (s *AuthService) Setup(ctx context.Context, in SetupInput) error {
	in.Name1 = strings.TrimSpace(in.Name1)
	in.Name2 = strings.TrimSpace(in.Name2)
	switch {
	case in.Name1 == "" || in.Name2 == "":
		return fmt.Errorf("%w: both profile names are required", domain.ErrValidation)
	case strings.EqualFold(in.Name1, in.Name2):
		return fmt.Errorf("%w: profile names must differ", domain.ErrValidation)
	case !validPIN(in.Pin1) || !validPIN(in.Pin2):
		return fmt.Errorf("%w: PIN must be %d-%d digits", domain.ErrValidation, pinMinLength, pinMaxLength)
	}
	now := s.now()
	first, err := newUser(in.Name1, in.Pin1, now)
	if err != nil {
		return err
	}
	second, err := newUser(in.Name2, in.Pin2, now)
	if err != nil {
		return err
	}
	return s.users.Setup(ctx, domain.SystemActor(), first, second)
}

// Login verifies a PIN and returns a raw session token. Unknown user and wrong
// PIN share one opaque error so neither can be probed.
func (s *AuthService) Login(ctx context.Context, name, pin string) (string, error) {
	user, err := s.users.FindByName(ctx, domain.SystemActor(), strings.TrimSpace(name))
	if err != nil || !domain.VerifyPIN(pin, user.PinHash, user.PinSalt, user.PinIter) {
		return "", domain.ErrForbidden
	}
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	now := s.now()
	session := domain.Session{
		TokenHash: domain.HashToken(token),
		UserID:    user.ID,
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}
	if err := s.sessions.Create(ctx, domain.NewActor(user.ID), session); err != nil {
		return "", err
	}
	return token, nil
}

// Authenticate resolves a raw token to an actor. Expired sessions are revoked
// and treated as anonymous.
func (s *AuthService) Authenticate(ctx context.Context, token string) (domain.Actor, error) {
	if token == "" {
		return domain.Actor{}, domain.ErrForbidden
	}
	session, err := s.sessions.FindByTokenHash(ctx, domain.SystemActor(), domain.HashToken(token))
	if err != nil {
		return domain.Actor{}, err
	}
	if !session.ExpiresAt.After(s.now()) {
		_ = s.sessions.Delete(ctx, domain.SystemActor(), session.TokenHash)
		return domain.Actor{}, domain.ErrForbidden
	}
	return domain.NewActor(session.UserID), nil
}

// Logout revokes the session that backs the given token.
func (s *AuthService) Logout(ctx context.Context, a domain.Actor, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.Delete(ctx, a, domain.HashToken(token))
}

func newUser(name, pin string, now time.Time) (domain.User, error) {
	hash, salt, iter, err := domain.HashPIN(pin)
	if err != nil {
		return domain.User{}, err
	}
	return domain.User{Name: name, PinHash: hash, PinSalt: salt, PinIter: iter, CreatedAt: now}, nil
}

func validPIN(pin string) bool {
	if len(pin) < pinMinLength || len(pin) > pinMaxLength {
		return false
	}
	for _, r := range pin {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func randomToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
