// Package mysql implements the identity repositories against MySQL.
package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"

	"identity-service/services/identity/internal/domain"
)

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

// CreateUser inserts the user, its phone credential, its profile, and a
// `user.created` outbox row in ONE transaction (transactional outbox, PRD §4.2).
func (r *UserRepo) CreateUser(ctx context.Context, phone string, profile domain.Profile, status domain.Status) (*domain.User, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `INSERT INTO users (status) VALUES (?)`, string(status))
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	userID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO user_credentials (user_id, auth_type, identifier) VALUES (?, ?, ?)`,
		userID, string(domain.AuthTypePhone), phone)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 { // duplicate key
			return nil, domain.ErrCredentialConflict
		}
		return nil, fmt.Errorf("insert credential: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO user_profiles (user_id, nickname, profile_image_url) VALUES (?, ?, ?)`,
		userID, profile.Nickname, nullable(profile.ProfileImageURL))
	if err != nil {
		return nil, fmt.Errorf("insert profile: %w", err)
	}

	payload, err := json.Marshal(map[string]any{
		"user_id":  userID,
		"nickname": profile.Nickname,
	})
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO outbox (aggregate_type, aggregate_id, event_type, payload) VALUES ('user', ?, 'user.created', ?)`,
		fmt.Sprintf("%d", userID), payload)
	if err != nil {
		return nil, fmt.Errorf("insert outbox: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	now := time.Now()
	return &domain.User{ID: userID, Status: status, CreatedAt: now, UpdatedAt: now}, nil
}

// FindByCredential resolves a user through a credential (e.g. PHONE + number).
// Returns domain.ErrUserNotFound when no such credential exists.
func (r *UserRepo) FindByCredential(ctx context.Context, authType domain.AuthType, identifier string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT u.id, u.status, u.created_at, u.updated_at, u.last_login_at
		FROM user_credentials c
		JOIN users u ON u.id = c.user_id
		WHERE c.auth_type = ? AND c.identifier = ?`,
		string(authType), identifier)

	var u domain.User
	var status string
	var lastLogin sql.NullTime
	if err := row.Scan(&u.ID, &status, &u.CreatedAt, &u.UpdatedAt, &lastLogin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	u.Status = domain.Status(status)
	if lastLogin.Valid {
		u.LastLoginAt = &lastLogin.Time
	}
	return &u, nil
}

// FindByID returns domain.ErrUserNotFound when the user does not exist.
func (r *UserRepo) FindByID(ctx context.Context, userID int64) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, status, created_at, updated_at, last_login_at FROM users WHERE id = ?`, userID)
	var u domain.User
	var status string
	var lastLogin sql.NullTime
	if err := row.Scan(&u.ID, &status, &u.CreatedAt, &u.UpdatedAt, &lastLogin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	u.Status = domain.Status(status)
	if lastLogin.Valid {
		u.LastLoginAt = &lastLogin.Time
	}
	return &u, nil
}

func (r *UserRepo) GetProfile(ctx context.Context, userID int64) (*domain.Profile, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT user_id, nickname, COALESCE(profile_image_url, ''), reputation_score
		 FROM user_profiles WHERE user_id = ?`, userID)
	var p domain.Profile
	if err := row.Scan(&p.UserID, &p.Nickname, &p.ProfileImageURL, &p.ReputationScore); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *UserRepo) UpdateStatus(ctx context.Context, userID int64, status domain.Status) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET status = ? WHERE id = ?`, string(status), userID)
	return err
}

func (r *UserRepo) TouchLastLogin(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET last_login_at = NOW(6) WHERE id = ?`, userID)
	return err
}

// UpsertDevice records the device and reports whether it was already known.
func (r *UserRepo) UpsertDevice(ctx context.Context, userID int64, fingerprint, name string) (known bool, err error) {
	if fingerprint == "" {
		return true, nil // clients without fingerprints skip device tracking (PoC)
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO user_devices (user_id, device_fingerprint, device_name)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE last_seen_at = NOW(6), device_name = VALUES(device_name)`,
		userID, fingerprint, nullable(name))
	if err != nil {
		return false, err
	}
	// MySQL reports 1 affected row for a fresh INSERT, 2 for a duplicate-key UPDATE.
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected != 1, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
