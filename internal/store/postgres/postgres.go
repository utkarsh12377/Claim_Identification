package postgres

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/utkarsh/claim-identification/internal/model"
	"github.com/utkarsh/claim-identification/internal/store"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Store struct {
	pool *pgxpool.Pool
}

var _ store.Store = (*Store)(nil)

func Connect(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	s := &Store{pool: pool}
	if err := s.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	entries, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(entries)

	for _, name := range entries {
		sqlBytes, err := migrationFS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := s.pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) UpsertProduct(ctx context.Context, p *model.Product) error {
	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encode product: %w", err)
	}

	const q = `
		INSERT INTO products (id, mcr_id, sku, title, brand, status, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE
		SET mcr_id     = EXCLUDED.mcr_id,
		    sku        = EXCLUDED.sku,
		    title      = EXCLUDED.title,
		    brand      = EXCLUDED.brand,
		    status     = EXCLUDED.status,
		    data       = EXCLUDED.data,
		    updated_at = now()`

	if _, err := s.pool.Exec(ctx, q, p.ID, p.McrID, p.SKU, p.Title, p.Brand, p.Status, data); err != nil {
		return fmt.Errorf("upsert product %s: %w", p.ID, err)
	}
	return nil
}

func (s *Store) GetProduct(ctx context.Context, id string) (*model.Product, error) {
	const q = `SELECT data FROM products WHERE id = $1`

	var data []byte
	if err := s.pool.QueryRow(ctx, q, id).Scan(&data); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, store.ErrProductNotFound
		}
		return nil, fmt.Errorf("get product %s: %w", id, err)
	}

	var p model.Product
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("decode product %s: %w", id, err)
	}
	return &p, nil
}

func (s *Store) CreateWorkflow(ctx context.Context, w *model.Workflow) error {
	steps, err := json.Marshal(w.Steps)
	if err != nil {
		return fmt.Errorf("encode workflow steps: %w", err)
	}

	const q = `
		INSERT INTO workflows (id, product_id, status, steps, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	if _, err := s.pool.Exec(ctx, q, w.ID, w.ProductID, string(w.Status), steps, w.CreatedAt, w.UpdatedAt); err != nil {
		return fmt.Errorf("create workflow %s: %w", w.ID, err)
	}
	return nil
}

func (s *Store) GetWorkflow(ctx context.Context, id string) (*model.Workflow, error) {
	const q = `
		SELECT id, product_id, status, COALESCE(error, ''), steps, result,
		       created_at, updated_at, completed_at
		FROM workflows
		WHERE id = $1`

	var (
		w           model.Workflow
		status      string
		steps       []byte
		result      []byte
		completedAt *time.Time
	)

	err := s.pool.QueryRow(ctx, q, id).Scan(
		&w.ID, &w.ProductID, &status, &w.Error, &steps, &result,
		&w.CreatedAt, &w.UpdatedAt, &completedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, store.ErrWorkflowNotFound
		}
		return nil, fmt.Errorf("get workflow %s: %w", id, err)
	}

	w.Status = model.WorkflowStatus(status)
	w.CompletedAt = completedAt

	if len(steps) > 0 {
		if err := json.Unmarshal(steps, &w.Steps); err != nil {
			return nil, fmt.Errorf("decode workflow steps %s: %w", id, err)
		}
	}
	if len(result) > 0 {
		var product model.Product
		if err := json.Unmarshal(result, &product); err != nil {
			return nil, fmt.Errorf("decode workflow result %s: %w", id, err)
		}
		w.Product = &product
	}
	return &w, nil
}

func (s *Store) CompleteWorkflow(ctx context.Context, w *model.Workflow, enriched *model.Product) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if err := closeRun(ctx, tx, w, model.WorkflowStatusCompleted, enriched); err != nil {
			return err
		}

		data, err := json.Marshal(enriched)
		if err != nil {
			return fmt.Errorf("encode enriched product: %w", err)
		}
		const updateProduct = `UPDATE products SET data = $2, updated_at = now() WHERE id = $1`
		if _, err := tx.Exec(ctx, updateProduct, enriched.ID, data); err != nil {
			return fmt.Errorf("update product %s: %w", enriched.ID, err)
		}

		const deleteClaims = `DELETE FROM claims WHERE product_id = $1`
		if _, err := tx.Exec(ctx, deleteClaims, enriched.ID); err != nil {
			return fmt.Errorf("clear claims for %s: %w", enriched.ID, err)
		}

		const insertClaim = `
			INSERT INTO claims (id, product_id, workflow_id, claim_type, claim_value, status, source, evidence, rule_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
		for _, c := range enriched.Claims {
			_, err := tx.Exec(ctx, insertClaim,
				c.ID, enriched.ID, w.ID, string(c.ClaimType), c.ClaimValue,
				string(c.Status), c.Source, c.Evidence, c.RuleID)
			if err != nil {
				return fmt.Errorf("insert claim %s: %w", c.ID, err)
			}
		}
		return nil
	})
}

func (s *Store) FailWorkflow(ctx context.Context, w *model.Workflow) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		return closeRun(ctx, tx, w, model.WorkflowStatusFailed, nil)
	})
}

// the status guard in the WHERE clause makes a second close a no-op
func closeRun(ctx context.Context, tx pgx.Tx, w *model.Workflow, status model.WorkflowStatus, result *model.Product) error {
	steps, err := json.Marshal(w.Steps)
	if err != nil {
		return fmt.Errorf("encode workflow steps: %w", err)
	}

	var resultJSON []byte
	if result != nil {
		if resultJSON, err = json.Marshal(result); err != nil {
			return fmt.Errorf("encode workflow result: %w", err)
		}
	}

	const q = `
		UPDATE workflows
		SET status = $2, error = NULLIF($3, ''), steps = $4, result = $5,
		    updated_at = now(), completed_at = now()
		WHERE id = $1 AND status = $6`

	tag, err := tx.Exec(ctx, q, w.ID, string(status), w.Error, steps, resultJSON, string(model.WorkflowStatusInProgress))
	if err != nil {
		return fmt.Errorf("close workflow %s: %w", w.ID, err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		const check = `SELECT EXISTS (SELECT 1 FROM workflows WHERE id = $1)`
		if err := tx.QueryRow(ctx, check, w.ID).Scan(&exists); err != nil {
			return fmt.Errorf("check workflow %s: %w", w.ID, err)
		}
		if !exists {
			return store.ErrWorkflowNotFound
		}
		return store.ErrInvalidTransition
	}
	return nil
}

func (s *Store) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	s.pool.Close()
	return nil
}
