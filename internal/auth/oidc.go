package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/unmaykr-a/transit-tracker/internal/config"
	"github.com/unmaykr-a/transit-tracker/internal/store/db"
)

var (
	ErrOIDCNoAccount = errors.New("no account is linked to this sign-in and automatic sign-up is off")
	ErrOIDCDisabled  = errors.New("this account is disabled")
)

// OIDC signs users in with an OpenID Connect provider (authorization code
// flow with PKCE). Discovery happens on first use and is retried until it
// succeeds, so a provider that is down at startup does not break the server.
type OIDC struct {
	cfg         config.OIDCConfig
	redirectURL string

	mu       sync.Mutex
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
}

func NewOIDC(cfg config.OIDCConfig, redirectURL string) *OIDC {
	return &OIDC{cfg: cfg, redirectURL: redirectURL}
}

// Name labels the sign-in button.
func (o *OIDC) Name() string { return o.cfg.Name }

func (o *OIDC) load(ctx context.Context) (*oidc.Provider, *oidc.IDTokenVerifier, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.provider == nil {
		p, err := oidc.NewProvider(ctx, o.cfg.Issuer)
		if err != nil {
			// The issuer must match the provider's exactly, and providers differ on the
			// trailing slash (Authentik has one): retry with it toggled before giving up.
			alt := strings.TrimSuffix(o.cfg.Issuer, "/")
			if alt == o.cfg.Issuer {
				alt += "/"
			}
			var altErr error
			if p, altErr = oidc.NewProvider(ctx, alt); altErr != nil {
				return nil, nil, fmt.Errorf("oidc discovery: %w", err)
			}
		}
		o.provider = p
		o.verifier = p.Verifier(&oidc.Config{ClientID: o.cfg.ClientID})
	}
	return o.provider, o.verifier, nil
}

func (o *OIDC) oauth(p *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID: o.cfg.ClientID, ClientSecret: o.cfg.ClientSecret, RedirectURL: o.redirectURL,
		Endpoint: p.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "profile", "email", "groups"},
	}
}

// Flow is what must survive the round trip to the provider (kept in a signed cookie).
type Flow struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	Next     string `json:"x"`
}

// Start begins a sign-in and returns the provider URL to redirect to.
func (o *OIDC) Start(ctx context.Context, next string) (string, Flow, error) {
	p, _, err := o.load(ctx)
	if err != nil {
		return "", Flow{}, err
	}
	f := Flow{State: NewToken(""), Nonce: NewToken(""), Verifier: oauth2.GenerateVerifier(), Next: next}
	url := o.oauth(p).AuthCodeURL(f.State, oidc.Nonce(f.Nonce), oauth2.S256ChallengeOption(f.Verifier))
	return url, f, nil
}

// Claims are the identity facts we use from the ID token (and userinfo).
type Claims struct {
	Issuer            string   `json:"iss"`
	Subject           string   `json:"sub"`
	Email             string   `json:"email"`
	EmailVerified     bool     `json:"email_verified"`
	PreferredUsername string   `json:"preferred_username"`
	Name              string   `json:"name"`
	Groups            []string `json:"groups"`
}

// Finish exchanges the code and verifies the ID token against the flow.
func (o *OIDC) Finish(ctx context.Context, f Flow, state, code string) (Claims, error) {
	if state == "" || state != f.State {
		return Claims{}, errors.New("sign-in state mismatch; start again")
	}
	p, verifier, err := o.load(ctx)
	if err != nil {
		return Claims{}, err
	}
	tok, err := o.oauth(p).Exchange(ctx, code, oauth2.VerifierOption(f.Verifier))
	if err != nil {
		return Claims{}, fmt.Errorf("oidc token exchange: %w", err)
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		return Claims{}, errors.New("oidc: no id_token in token response")
	}
	idt, err := verifier.Verify(ctx, raw)
	if err != nil {
		return Claims{}, fmt.Errorf("oidc: %w", err)
	}
	if idt.Nonce != f.Nonce {
		return Claims{}, errors.New("oidc: nonce mismatch")
	}
	var c Claims
	if err := idt.Claims(&c); err != nil {
		return Claims{}, fmt.Errorf("oidc claims: %w", err)
	}
	// Some providers only put groups in userinfo.
	if len(c.Groups) == 0 && o.cfg.AdminGroup != "" {
		if ui, err := p.UserInfo(ctx, oauth2.StaticTokenSource(tok)); err == nil {
			var extra Claims
			if ui.Claims(&extra) == nil {
				c.Groups = extra.Groups
			}
		}
	}
	c.Issuer, c.Subject = idt.Issuer, idt.Subject
	return c, nil
}

var usernameStrip = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// baseUsername turns a claim into something ValidateUsername accepts.
func baseUsername(c Claims) string {
	for _, cand := range []string{c.PreferredUsername, strings.Split(c.Email, "@")[0], c.Name} {
		u := strings.Trim(usernameStrip.ReplaceAllString(cand, "-"), "-._")
		if len(u) > 28 {
			u = u[:28]
		}
		if len(u) >= 3 {
			return u
		}
	}
	sum := sha256.Sum256([]byte(c.Issuer + "\x00" + c.Subject))
	return "user-" + base64.RawURLEncoding.EncodeToString(sum[:])[:8]
}

// SignInOIDC returns the local user for verified claims: the linked one, or
// a new user when auto-creation is on. With an admin group configured, the
// role follows group membership on every sign-in (the last admin is never demoted).
func (s *Service) SignInOIDC(ctx context.Context, cfg config.OIDCConfig, c Claims) (db.User, error) {
	var u db.User
	now := s.now().Unix()
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		row, err := q.GetIdentityUser(ctx, db.GetIdentityUserParams{Issuer: c.Issuer, Subject: c.Subject})
		switch {
		case err == nil:
			u = row.User
			if err := q.TouchIdentity(ctx, db.TouchIdentityParams{LastLogin: now, Email: c.Email, Issuer: c.Issuer, Subject: c.Subject}); err != nil {
				return err
			}
		case errors.Is(err, sql.ErrNoRows):
			if !cfg.AutoCreate {
				return ErrOIDCNoAccount
			}
			n, err := q.CountUsers(ctx)
			if err != nil {
				return err
			}
			role := RoleUser
			if n == 0 { // the very first user administers the instance, as with /setup
				role = RoleAdmin
			}
			if cfg.AdminGroup != "" && slices.Contains(c.Groups, cfg.AdminGroup) {
				role = RoleAdmin
			}
			name := baseUsername(c)
			for i := 2; ; i++ {
				if _, err := q.GetUserByUsername(ctx, name); errors.Is(err, sql.ErrNoRows) {
					break
				} else if err != nil {
					return err
				}
				name = fmt.Sprintf("%s-%d", strings.TrimRight(baseUsername(c), "-"), i)
			}
			email := ""
			if c.EmailVerified {
				email = c.Email
			}
			if u, err = q.CreateUserNoPassword(ctx, db.CreateUserNoPasswordParams{Username: name, Email: email, Role: role, CreatedAt: now}); err != nil {
				return err
			}
			if err := q.CreateIdentity(ctx, db.CreateIdentityParams{Issuer: c.Issuer, Subject: c.Subject, UserID: u.ID, Email: c.Email, CreatedAt: now, LastLogin: now}); err != nil {
				return err
			}
		default:
			return err
		}
		if u.Disabled != 0 {
			return ErrOIDCDisabled
		}
		if cfg.AdminGroup != "" {
			want := RoleUser
			if slices.Contains(c.Groups, cfg.AdminGroup) {
				want = RoleAdmin
			}
			if want != u.Role {
				if want == RoleUser {
					if err := s.GuardLastAdmin(ctx, q, u, RoleUser, false); err != nil {
						return nil // keep the last admin rather than lock everyone out
					}
				}
				if err := q.UpdateUserRole(ctx, db.UpdateUserRoleParams{Role: want, ID: u.ID}); err != nil {
					return err
				}
				u.Role = want
			}
		}
		return nil
	})
	return u, err
}

// randomKey is a per-process key for signing short-lived flow cookies.
func randomKey() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// FlowKey signs OIDC flow cookies; a restart only invalidates sign-ins in progress.
var FlowKey = randomKey()
