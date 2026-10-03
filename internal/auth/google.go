package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type GoogleAuthenticator struct {
	config    *oauth2.Config
	verifier  *oidc.IDTokenVerifier
	allowlist Allowlist
}

// Allowlist admits accounts by exact email address or by email domain.
type Allowlist struct {
	emails  map[string]struct{}
	domains map[string]struct{}
}

func NewAllowlist(emails, domains []string) Allowlist {
	a := Allowlist{emails: map[string]struct{}{}, domains: map[string]struct{}{}}
	for _, email := range emails {
		if email = strings.ToLower(strings.TrimSpace(email)); email != "" {
			a.emails[email] = struct{}{}
		}
	}
	for _, domain := range domains {
		if domain = strings.ToLower(strings.TrimSpace(domain)); domain != "" {
			a.domains[domain] = struct{}{}
		}
	}
	return a
}

// Allowed reports whether email is on the allowlist, either exactly or by its
// domain. Sessions are re-checked with it on every request, so removing an
// address or domain revokes existing sessions.
func (a Allowlist) Allowed(email string) bool {
	_, ok := a.match(email)
	return ok
}

// AllowedClaims is the login check. A domain entry only admits an account
// whose Google hd (hosted domain) claim names that domain: the email suffix
// alone does not prove that the domain's Google Workspace manages the account.
func (a Allowlist) AllowedClaims(claims *GoogleClaims) bool {
	if claims == nil {
		return false
	}
	domain, ok := a.match(claims.Email)
	if !ok {
		return false
	}
	return domain == "" || strings.EqualFold(strings.TrimSpace(claims.HostedDomain), domain)
}

// match returns ("", true) for an exact email entry, or (domain, true) when
// only the email's domain is listed.
func (a Allowlist) match(email string) (string, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "", false
	}
	if _, ok := a.emails[email]; ok {
		return "", true
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "", false
	}
	if _, ok := a.domains[parts[1]]; ok {
		return parts[1], true
	}
	return "", false
}

func NewGoogleAuthenticator(ctx context.Context, clientID, clientSecret, redirectURL string, allowlist Allowlist) (*GoogleAuthenticator, error) {
	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return nil, fmt.Errorf("oidc provider: %w", err)
	}

	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     google.Endpoint,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}

	return &GoogleAuthenticator{
		config:    config,
		verifier:  provider.Verifier(&oidc.Config{ClientID: clientID}),
		allowlist: allowlist,
	}, nil
}

func (g *GoogleAuthenticator) AuthURL(state string) string {
	return g.config.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "select_account"),
	)
}

func (g *GoogleAuthenticator) Exchange(ctx context.Context, code string) (*GoogleClaims, error) {
	token, err := g.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in response")
	}

	idToken, err := g.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verify id_token: %w", err)
	}

	var claims GoogleClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	return &claims, nil
}

func (g *GoogleAuthenticator) IsAllowed(claims *GoogleClaims) bool {
	return g.allowlist.AllowedClaims(claims)
}

func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
