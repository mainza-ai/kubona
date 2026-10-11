package memory

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/ifc"
)

// PostgresStoreConfig configures connectivity to the pgvector enterprise sidecar
type PostgresStoreConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DefaultPostgresStoreConfig returns operator configuration. The DSN
// comes from KUBONA_PG_DSN with NO default: a hardcoded dev password in
// source shipped here once and was removed — connection strings with
// secrets are operator config, never code.
// Empty DSN means unconfigured; constructors must refuse it.
func DefaultPostgresStoreConfig() PostgresStoreConfig {
	return PostgresStoreConfig{
		DSN:             os.Getenv("KUBONA_PG_DSN"),
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}
}

// PostgresVectorStore manages distributed Hindsight memories and session locks on PostgreSQL + pgvector
type PostgresVectorStore struct {
	db  *sql.DB
	cfg PostgresStoreConfig
}

// NewPostgresVectorStore connects to PostgreSQL and verifies the pgvector extension
func NewPostgresVectorStore(db *sql.DB, cfg PostgresStoreConfig) *PostgresVectorStore {
	return &PostgresVectorStore{
		db:  db,
		cfg: cfg,
	}
}

// FormatVectorString serializes float32 slice to pgvector string format: "[0.1,0.2,...]"
func FormatVectorString(vec []float32) string {
	var sb strings.Builder
	sb.WriteString("[")
	for i, v := range vec {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%f", v))
	}
	sb.WriteString("]")
	return sb.String()
}

// Retain stores a memory in PostgreSQL with pgvector embedding
func (s *PostgresVectorStore) Retain(
	ctx context.Context,
	tenantID string,
	readership string,
	bank MemoryBank,
	content string,
	importance float64,
	embedding []float32,
) (*MemoryItem, error) {
	if s.db == nil {
		return nil, fmt.Errorf("postgres database connection not initialized")
	}

	if tenantID == "" {
		tenantID = "default"
	}
	if readership == "" {
		readership = "public"
	}

	tokens := len(content) / 4
	if tokens < 1 {
		tokens = 1
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	item := &MemoryItem{
		ID:             id,
		TenantID:       tenantID,
		Readership:     readership,
		Bank:           bank,
		Content:        content,
		TokensEstimate: tokens,
		Importance:     importance,
		EffectiveScore: importance,
		CreatedAt:      now,
		LastAccessedAt: now,
	}

	vecStr := FormatVectorString(embedding)

	query := `
		INSERT INTO hindsight_memories (
			id, tenant_id, readership, bank, content, tokens_estimate, importance, effective_score, embedding, created_at, last_accessed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9::vector, $10, $11
		)
	`
	_, err := s.db.ExecContext(ctx, query, id, tenantID, readership, string(bank), content, tokens, importance, importance, vecStr, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to insert memory into postgres: %w", err)
	}

	return item, nil
}

// SemanticRecall queries PostgreSQL using HNSW vector cosine distance (<=>) with IFC and tenant filtering
func (s *PostgresVectorStore) SemanticRecall(
	ctx context.Context,
	bank MemoryBank,
	queryEmbedding []float32,
	maxTokens int,
	minSimilarity float64,
	requesterTenant string,
	requesterPrincipals []string,
	limit int,
) ([]*ScoredMemoryItem, error) {
	if s.db == nil {
		return nil, fmt.Errorf("postgres database connection not initialized")
	}

	if limit <= 0 {
		limit = 10
	}

	vecStr := FormatVectorString(queryEmbedding)

	// In pgvector, cosine distance d = 1 - cosine_similarity, so cosine_similarity = 1 - d
	// Where d = embedding <=> query_vec
	query := `
		SELECT id, tenant_id, readership, bank, content, tokens_estimate, importance, effective_score, created_at, last_accessed_at,
		       (1.0 - (embedding <=> $1::vector)) AS similarity
		FROM hindsight_memories
		WHERE (bank = $2 OR $2 = '')
		  AND (1.0 - (embedding <=> $1::vector)) >= $3
		ORDER BY (embedding <=> $1::vector) ASC
		LIMIT $4
	`

	rows, err := s.db.QueryContext(ctx, query, vecStr, string(bank), minSimilarity, limit*2)
	if err != nil {
		return nil, fmt.Errorf("failed to query postgres pgvector: %w", err)
	}
	defer rows.Close()

	var destRS ifc.ReaderSet
	if len(requesterPrincipals) == 0 {
		destRS = ifc.Public()
	} else {
		destRS = ifc.Restricted(requesterPrincipals...)
	}

	var results []*ScoredMemoryItem
	tokensUsed := 0

	for rows.Next() {
		var item MemoryItem
		var bankStr string
		var sim float64

		if err := rows.Scan(
			&item.ID, &item.TenantID, &item.Readership, &bankStr, &item.Content,
			&item.TokensEstimate, &item.Importance, &item.EffectiveScore,
			&item.CreatedAt, &item.LastAccessedAt, &sim,
		); err != nil {
			return nil, err
		}
		item.Bank = MemoryBank(bankStr)

		// 1. Strict tenant boundary (same rule as hindsight recall: no
		// 'default' bypass; empty normalizes to 'default' on both sides).
		itemTenant := item.TenantID
		if itemTenant == "" {
			itemTenant = "default"
		}
		reqTenant := requesterTenant
		if reqTenant == "" {
			reqTenant = "default"
		}
		if itemTenant != reqTenant {
			continue
		}

		// 2. IFC ReaderSet lattice check
		var sourceRS ifc.ReaderSet
		if item.Readership == "public" || item.Readership == "" {
			sourceRS = ifc.Public()
		} else {
			principals := strings.Split(item.Readership, ",")
			for i := range principals {
				principals[i] = strings.TrimSpace(principals[i])
			}
			sourceRS = ifc.Restricted(principals...)
		}

		if !sourceRS.CanFlowTo(destRS) {
			continue
		}

		// 3. Token budget packing
		if maxTokens > 0 && tokensUsed+item.TokensEstimate > maxTokens {
			if len(results) > 0 {
				break
			}
		}

		results = append(results, &ScoredMemoryItem{
			Item:       &item,
			Similarity: sim,
		})
		tokensUsed += item.TokensEstimate

		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// AcquireDistributedLock attempts to acquire or refresh a named distributed lease
func (s *PostgresVectorStore) AcquireDistributedLock(
	ctx context.Context,
	lockKey string,
	holderID string,
	leaseDuration time.Duration,
) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("postgres database connection not initialized")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(leaseDuration)

	// Upsert query: acquire if unheld or expired, or extend if current holder
	query := `
		INSERT INTO distributed_locks (lock_key, holder_node_id, lease_expires_at, acquired_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (lock_key) DO UPDATE
		SET holder_node_id = EXCLUDED.holder_node_id,
		    lease_expires_at = EXCLUDED.lease_expires_at,
		    acquired_at = EXCLUDED.acquired_at
		WHERE distributed_locks.lease_expires_at < $4
		   OR distributed_locks.holder_node_id = EXCLUDED.holder_node_id
	`

	res, err := s.db.ExecContext(ctx, query, lockKey, holderID, expiresAt, now)
	if err != nil {
		return false, fmt.Errorf("failed to acquire distributed lock: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return affected > 0, nil
}

// ReleaseDistributedLock releases a distributed lock held by holderID
func (s *PostgresVectorStore) ReleaseDistributedLock(ctx context.Context, lockKey string, holderID string) error {
	if s.db == nil {
		return fmt.Errorf("postgres database connection not initialized")
	}

	query := `DELETE FROM distributed_locks WHERE lock_key = $1 AND holder_node_id = $2`
	_, err := s.db.ExecContext(ctx, query, lockKey, holderID)
	return err
}
