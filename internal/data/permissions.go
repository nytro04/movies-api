package data

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// Permission slice to hold the permissions for a user eg. "movies:read"
type Permissions []string

// Include checks if a permission code is included in the Permissions slice
func (p Permissions) Include(code string) bool {
	for i := range p {
		if p[i] == code {
			return true
		}
	}
	return false
}

// PermissionModel defines the structure for the permission model
type PermissionModel struct {
	DB *sql.DB
}

// GetAllForUser returns all permissions for a specific user
func (m PermissionModel) GetAllForUser(userId int64) (Permissions, error) {
	query := `
		SELECT permissions.code
		FROM permissions
		INNER JOIN users_permissions ON users_permissions.permission_id = permissions.id
		INNER JOIN users ON users_permissions.user_id = users.id
		WHERE users.id = $1
		`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := m.DB.QueryContext(ctx, query, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions Permissions

	for rows.Next() {
		var permission string

		err := rows.Scan(&permission)
		if err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return permissions, nil
}

func (m PermissionModel) AddForUser(userID int64, codes ...string) error {
	query := `
		INSERT INTO users_permissions
		SELECT $1, permissions.id FROM permissions WHERE permissions.code = ANY($2)`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := m.DB.ExecContext(ctx, query, userID, pq.Array(codes))
	return err
}

// Mock data for testing
type MockPermissionModel struct{}

func (m MockPermissionModel) GetAllForUser(userId int64) (Permissions, error) {
	return Permissions{"movies:read", "movies:write"}, nil
}
