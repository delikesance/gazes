package auth

import (
	"context"
	"database/sql"
)

// RoleByPseudo returns the role of the user with this pseudo. It returns sql.ErrNoRows when no
// user matches and ErrAmbiguousPseudo when several do.
func (s *Store) RoleByPseudo(ctx context.Context, pseudo string) (string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT role FROM users WHERE pseudo = ? LIMIT 2`, pseudo)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return "", err
		}
		roles = append(roles, r)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	switch len(roles) {
	case 0:
		return "", sql.ErrNoRows
	case 1:
		return roles[0], nil
	}
	return "", ErrAmbiguousPseudo
}

// UserRole returns the role of the user with this ID (sql.ErrNoRows when unknown).
func (s *Store) UserRole(ctx context.Context, id int64) (string, error) {
	var r string
	err := s.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, id).Scan(&r)
	return r, err
}

// UserRole returns the role of the user with this ID (sql.ErrNoRows when unknown).
func (s *Service) UserRole(ctx context.Context, id int64) (string, error) {
	return s.store.UserRole(ctx, id)
}
