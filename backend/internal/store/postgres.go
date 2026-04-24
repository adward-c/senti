package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"senti/backend/internal/domain"
)

type Repository interface {
	EnsureSchema(ctx context.Context) error
	SeedInviteCode(ctx context.Context, code string) error
	CreateUser(ctx context.Context, user domain.User) error
	CreateUserWithInvite(ctx context.Context, user domain.User, inviteCode string) error
	GetUserByUsername(ctx context.Context, username string) (domain.User, error)
	CreateAnalysis(ctx context.Context, userID string, record domain.AnalysisRecord) error
	ListAnalyses(ctx context.Context, userID string, limit int) ([]domain.AnalysisSummary, error)
	GetAnalysis(ctx context.Context, userID string, id string) (domain.AnalysisRecord, error)
	DeleteAnalysis(ctx context.Context, userID string, id string) (string, error)
	Ping(ctx context.Context) error
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrInviteInvalid = errors.New("invite invalid")

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

func (r *PostgresRepository) EnsureSchema(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS invite_codes (
			code TEXT PRIMARY KEY,
			used_by TEXT,
			used_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS analyses (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			input_type TEXT NOT NULL,
			source_text TEXT NOT NULL,
			image_path TEXT,
			structured_messages JSONB NOT NULL DEFAULT '[]'::jsonb,
			metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
			result JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		ALTER TABLE analyses ADD COLUMN IF NOT EXISTS user_id TEXT;
		CREATE INDEX IF NOT EXISTS analyses_created_at_idx ON analyses (created_at DESC);
		CREATE INDEX IF NOT EXISTS analyses_user_created_at_idx ON analyses (user_id, created_at DESC);
	`)
	return err
}

func (r *PostgresRepository) SeedInviteCode(ctx context.Context, code string) error {
	if code == "" {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO invite_codes (code)
		VALUES ($1)
		ON CONFLICT (code) DO NOTHING
	`, code)
	return err
}

func (r *PostgresRepository) CreateUser(ctx context.Context, user domain.User) error {
	_, err := r.pool.Exec(
		ctx,
		`INSERT INTO users (id, username, password_hash, created_at) VALUES ($1, $2, $3, $4)`,
		user.ID,
		user.Username,
		user.PasswordHash,
		user.CreatedAt,
	)
	if err != nil {
		return err
	}
	return nil
}

func (r *PostgresRepository) CreateUserWithInvite(ctx context.Context, user domain.User, inviteCode string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE invite_codes
		SET used_by = $1, used_at = $2
		WHERE code = $3 AND used_at IS NULL
	`, user.ID, user.CreatedAt, inviteCode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrInviteInvalid
	}

	_, err = tx.Exec(
		ctx,
		`INSERT INTO users (id, username, password_hash, created_at) VALUES ($1, $2, $3, $4)`,
		user.ID,
		user.Username,
		user.PasswordHash,
		user.CreatedAt,
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) GetUserByUsername(ctx context.Context, username string) (domain.User, error) {
	row := r.pool.QueryRow(ctx, `SELECT id, username, password_hash, created_at FROM users WHERE username = $1`, username)
	var user domain.User
	if err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, ErrNotFound
		}
		return domain.User{}, err
	}
	return user, nil
}

func (r *PostgresRepository) CreateAnalysis(ctx context.Context, userID string, record domain.AnalysisRecord) error {
	messagesJSON, err := json.Marshal(record.StructuredMessages)
	if err != nil {
		return err
	}
	metricsJSON, err := json.Marshal(record.Result.Metrics)
	if err != nil {
		return err
	}
	resultJSON, err := json.Marshal(record.Result)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(
		ctx,
		`INSERT INTO analyses (id, user_id, input_type, source_text, image_path, structured_messages, metrics, result, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8::jsonb, $9)`,
		record.ID,
		userID,
		record.InputType,
		record.SourceText,
		record.ImagePath,
		string(messagesJSON),
		string(metricsJSON),
		string(resultJSON),
		record.CreatedAt,
	)
	return err
}

func (r *PostgresRepository) ListAnalyses(ctx context.Context, userID string, limit int) ([]domain.AnalysisSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, input_type, result->>'stage' AS stage, result->>'summary' AS summary, created_at
		FROM analyses
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaries := make([]domain.AnalysisSummary, 0, limit)
	for rows.Next() {
		var item domain.AnalysisSummary
		if err := rows.Scan(&item.ID, &item.InputType, &item.Stage, &item.Summary, &item.CreatedAt); err != nil {
			return nil, err
		}
		summaries = append(summaries, item)
	}
	return summaries, rows.Err()
}

func (r *PostgresRepository) GetAnalysis(ctx context.Context, userID string, id string) (domain.AnalysisRecord, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, user_id, input_type, source_text, COALESCE(image_path, ''), structured_messages, result, created_at
		FROM analyses
		WHERE id = $1 AND user_id = $2
	`, id, userID)

	var (
		record       domain.AnalysisRecord
		messagesJSON []byte
		resultJSON   []byte
	)
	if err := row.Scan(&record.ID, &record.UserID, &record.InputType, &record.SourceText, &record.ImagePath, &messagesJSON, &resultJSON, &record.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.AnalysisRecord{}, ErrNotFound
		}
		return domain.AnalysisRecord{}, err
	}

	if err := json.Unmarshal(messagesJSON, &record.StructuredMessages); err != nil {
		return domain.AnalysisRecord{}, err
	}
	if err := json.Unmarshal(resultJSON, &record.Result); err != nil {
		return domain.AnalysisRecord{}, err
	}
	record.Saved = true
	return record, nil
}

func (r *PostgresRepository) DeleteAnalysis(ctx context.Context, userID string, id string) (string, error) {
	row := r.pool.QueryRow(ctx, `
		DELETE FROM analyses
		WHERE id = $1 AND user_id = $2
		RETURNING COALESCE(image_path, '')
	`, id, userID)
	var imagePath string
	if err := row.Scan(&imagePath); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return imagePath, nil
}
