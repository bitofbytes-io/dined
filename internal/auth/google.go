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
	config         *oauth2.Config
	verifier       *oidc.IDTokenVerifier
	allowedDomains map[string]struct{}
	allowedEmails  map[string]struct{}
}

func NewGoogleAuthenticator(ctx context.Context, clientID, clientSecret, redirectURL string, allowedDomains, allowedEmails []string) (*GoogleAuthenticator, error) {
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

	domains := make(map[string]struct{}, len(allowedDomains))
	for _, domain := range allowedDomains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain != "" {
			domains[domain] = struct{}{}
		}
	}

	emails := make(map[string]struct{}, len(allowedEmails))
	for _, email := range allowedEmails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email != "" {
			emails[email] = struct{}{}
		}
	}

	return &GoogleAuthenticator{
		config:         config,
		verifier:       provider.Verifier(&oidc.Config{ClientID: clientID}),
		allowedDomains: domains,
		allowedEmails:  emails,
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

func (g *GoogleAuthenticator) IsEmailAllowed(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	if _, ok := g.allowedEmails[email]; ok {
		return true
	}

	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		_, ok := g.allowedDomains[parts[1]]
		return ok
	}
	return false
}

func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
