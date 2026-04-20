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
	CreateAnalysis(ctx context.Context, record domain.AnalysisRecord) error
	ListAnalyses(ctx context.Context, limit int) ([]domain.AnalysisSummary, error)
	GetAnalysis(ctx context.Context, id string) (domain.AnalysisRecord, error)
	Ping(ctx context.Context) error
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

var ErrNotFound = errors.New("not found")

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

func (r *PostgresRepository) CreateAnalysis(ctx context.Context, record domain.AnalysisRecord) error {
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
		`INSERT INTO analyses (id, input_type, source_text, image_path, structured_messages, metrics, result, created_at)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7::jsonb, $8)`,
		record.ID,
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

func (r *PostgresRepository) ListAnalyses(ctx context.Context, limit int) ([]domain.AnalysisSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, input_type, result->>'stage' AS stage, result->>'summary' AS summary, created_at
		FROM analyses
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
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

func (r *PostgresRepository) GetAnalysis(ctx context.Context, id string) (domain.AnalysisRecord, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, input_type, source_text, COALESCE(image_path, ''), structured_messages, result, created_at
		FROM analyses
		WHERE id = $1
	`, id)

	var (
		record       domain.AnalysisRecord
		messagesJSON []byte
		resultJSON   []byte
	)
	if err := row.Scan(&record.ID, &record.InputType, &record.SourceText, &record.ImagePath, &messagesJSON, &resultJSON, &record.CreatedAt); err != nil {
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
	return record, nil
}
