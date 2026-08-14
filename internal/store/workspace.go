package store

import (
	"context"
	"fmt"

	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func (s *Store) CreateWorkspaceWithOwner(ctx context.Context, wsName, wsSlug, email, password, userName string) (*domain.Workspace, *domain.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, nil, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	var user domain.User
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, name)
		VALUES ($1, $2, $3)
		RETURNING id, email, password_hash, name, created_at
	`, email, string(hash), userName).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.CreatedAt)
	if err != nil {
		return nil, nil, fmt.Errorf("create user: %w", err)
	}

	var ws domain.Workspace
	err = tx.QueryRow(ctx, `
		INSERT INTO workspaces (name, slug)
		VALUES ($1, $2)
		RETURNING id, name, slug, created_at, updated_at
	`, wsName, wsSlug).Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.CreatedAt, &ws.UpdatedAt)
	if err != nil {
		return nil, nil, fmt.Errorf("create workspace: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO workspace_members (workspace_id, user_id, role)
		VALUES ($1, $2, 'owner')
	`, ws.ID, user.ID)
	if err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return &ws, &user, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u domain.User
	err := s.Pool.QueryRow(ctx, `
		SELECT id, email, password_hash, name, created_at FROM users WHERE email=$1
	`, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) ListWorkspacesForUser(ctx context.Context, userID uuid.UUID) ([]domain.Workspace, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT w.id, w.name, w.slug, w.created_at, w.updated_at
		FROM workspaces w
		JOIN workspace_members m ON m.workspace_id = w.id
		WHERE m.user_id=$1
		ORDER BY w.created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Workspace
	for rows.Next() {
		var w domain.Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.Slug, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) UserRole(ctx context.Context, workspaceID, userID uuid.UUID) (domain.Role, error) {
	var role string
	err := s.Pool.QueryRow(ctx, `
		SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2
	`, workspaceID, userID).Scan(&role)
	if err != nil {
		return "", err
	}
	return domain.Role(role), nil
}
