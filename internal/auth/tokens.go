package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/unmaykr-a/transit-tracker/internal/store/db"
)

// TokenPrefix marks API tokens so they are recognisable in configs and logs.
const TokenPrefix = "bus_"

// MaxTokensPerUser bounds how many API tokens one user may hold.
const MaxTokensPerUser = 20

var ErrTooManyTokens = errors.New("too many API tokens; revoke one first")

// tokenTouchEvery limits last-used writes (Home Assistant polls every 30 s).
const tokenTouchEvery = 60

// CreateToken issues an API token for userID. The token is returned once; only its hash is stored.
func (s *Service) CreateToken(ctx context.Context, userID int64, name string) (string, db.ApiToken, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 64 {
		return "", db.ApiToken{}, &ValidationError{"name", "use 1–64 characters"}
	}
	n, err := s.Store.Q.CountUserAPITokens(ctx, userID)
	if err != nil {
		return "", db.ApiToken{}, err
	}
	if n >= MaxTokensPerUser {
		return "", db.ApiToken{}, ErrTooManyTokens
	}
	token := NewToken(TokenPrefix)
	t, err := s.Store.Q.CreateAPIToken(ctx, db.CreateAPITokenParams{
		UserID: userID, Name: name, TokenHash: HashToken(token), Prefix: token[:len(TokenPrefix)+6], CreatedAt: s.now().Unix(),
	})
	return token, t, err
}

// AuthenticateToken resolves a bearer token; nil means unknown, or the user is disabled.
func (s *Service) AuthenticateToken(ctx context.Context, token, ip string) (*Principal, error) {
	if !strings.HasPrefix(token, TokenPrefix) {
		return nil, nil
	}
	row, err := s.Store.Q.GetAPITokenWithUser(ctx, HashToken(token))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if row.User.Disabled != 0 {
		return nil, nil
	}
	now := s.now().Unix()
	if !row.ApiToken.LastUsedAt.Valid || now-row.ApiToken.LastUsedAt.Int64 >= tokenTouchEvery || row.ApiToken.LastUsedIp != ip {
		if err := s.Store.Q.TouchAPIToken(ctx, db.TouchAPITokenParams{LastUsedAt: sql.NullInt64{Int64: now, Valid: true}, LastUsedIp: ip, ID: row.ApiToken.ID}); err != nil {
			return nil, err
		}
	}
	return &Principal{User: row.User, TokenID: row.ApiToken.ID}, nil
}
