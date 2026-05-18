package auth

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type MemoryRepository struct {
	mu              sync.RWMutex
	usersByID       map[uuid.UUID]User
	usersByOAuth    map[string]uuid.UUID
	usersByEmail    map[string]uuid.UUID
	sessionsByID    map[uuid.UUID]Session
	sessionHashes   map[string]uuid.UUID
	sessionHashByID map[uuid.UUID]string
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		usersByID:       make(map[uuid.UUID]User),
		usersByOAuth:    make(map[string]uuid.UUID),
		usersByEmail:    make(map[string]uuid.UUID),
		sessionsByID:    make(map[uuid.UUID]Session),
		sessionHashes:   make(map[string]uuid.UUID),
		sessionHashByID: make(map[uuid.UUID]string),
	}
}

func (r *MemoryRepository) FindUserByOAuth(ctx context.Context, provider, providerID string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.usersByOAuth[oauthKey(provider, providerID)]
	if !ok {
		return nil, nil
	}
	user := r.usersByID[id]
	return &user, nil
}

func (r *MemoryRepository) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.usersByEmail[email]
	if !ok {
		return nil, nil
	}
	user := r.usersByID[id]
	return &user, nil
}

func (r *MemoryRepository) CreateUser(ctx context.Context, user User) (User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.usersByID[user.ID] = user
	r.usersByEmail[user.Email] = user.ID
	r.usersByOAuth[oauthKey(user.OAuthProvider, user.OAuthProviderID)] = user.ID
	return user, nil
}

func (r *MemoryRepository) UpdateUserLogin(ctx context.Context, id uuid.UUID, name, avatarURL string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	user, ok := r.usersByID[id]
	if !ok {
		return nil
	}
	now := time.Now()
	user.Name = name
	user.AvatarURL = avatarURL
	user.LastLoginAt = now
	user.UpdatedAt = now
	r.usersByID[id] = user
	return nil
}

func (r *MemoryRepository) CreateSession(ctx context.Context, session Session, tokenHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionsByID[session.ID] = session
	r.sessionHashes[tokenHash] = session.ID
	r.sessionHashByID[session.ID] = tokenHash
	return nil
}

func (r *MemoryRepository) FindSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, *User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sessionID, ok := r.sessionHashes[tokenHash]
	if !ok {
		return nil, nil, nil
	}
	session := r.sessionsByID[sessionID]
	user := r.usersByID[session.UserID]
	return &session, &user, nil
}

func (r *MemoryRepository) DeleteSession(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if tokenHash, ok := r.sessionHashByID[id]; ok {
		delete(r.sessionHashes, tokenHash)
		delete(r.sessionHashByID, id)
	}
	delete(r.sessionsByID, id)
	return nil
}

func (r *MemoryRepository) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	var deleted int64
	for id, session := range r.sessionsByID {
		if now.After(session.ExpiresAt) {
			if tokenHash, ok := r.sessionHashByID[id]; ok {
				delete(r.sessionHashes, tokenHash)
				delete(r.sessionHashByID, id)
			}
			delete(r.sessionsByID, id)
			deleted++
		}
	}
	return deleted, nil
}

func oauthKey(provider, providerID string) string {
	return provider + "\x00" + providerID
}
