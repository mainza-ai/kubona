package memory

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/ifc"
)

type MemoryBank string

const (
	BankWorldFacts   MemoryBank = "world-facts"
	BankExperiences  MemoryBank = "experiences"
	BankMentalModels MemoryBank = "mental-models"
)

type MemoryItem struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	Readership     string     `json:"readership"` // "public" or comma-separated principal IDs
	Bank           MemoryBank `json:"bank"`
	Content        string     `json:"content"`
	TokensEstimate int        `json:"tokens_estimate"`
	Importance     float64    `json:"importance"` // 0.0 to 1.0
	EffectiveScore float64    `json:"effective_score"`
	CreatedAt      time.Time  `json:"created_at"`
	LastAccessedAt time.Time  `json:"last_accessed_at"`
}

type StoreConfig struct {
	MaxItemsPerBank  int           `json:"max_items_per_bank"`
	MaxTokensPerBank int           `json:"max_tokens_per_bank"`
	DecayInterval    time.Duration `json:"decay_interval"`
	DecayRatePerHour float64       `json:"decay_rate_per_hour"`
}

func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		MaxItemsPerBank:  500,
		MaxTokensPerBank: 250000,
		DecayInterval:    1 * time.Hour,
		DecayRatePerHour: 0.01, // 1% decay per hour
	}
}

type HindsightStore struct {
	mu      sync.RWMutex
	db      *sql.DB
	cfg     StoreConfig
	items   map[string]*MemoryItem
	vectors map[string][]float32
}

func NewHindsightStore() *HindsightStore {
	return NewPersistentHindsightStoreWithConfig(nil, DefaultStoreConfig())
}

func NewHindsightStoreWithConfig(cfg StoreConfig) *HindsightStore {
	return NewPersistentHindsightStoreWithConfig(nil, cfg)
}

func NewPersistentHindsightStore(db *sql.DB) *HindsightStore {
	return NewPersistentHindsightStoreWithConfig(db, DefaultStoreConfig())
}

func NewPersistentHindsightStoreWithConfig(db *sql.DB, cfg StoreConfig) *HindsightStore {
	if cfg.MaxItemsPerBank <= 0 {
		cfg.MaxItemsPerBank = 500
	}
	if cfg.MaxTokensPerBank <= 0 {
		cfg.MaxTokensPerBank = 250000
	}
	if cfg.DecayInterval <= 0 {
		cfg.DecayInterval = 1 * time.Hour
	}
	if cfg.DecayRatePerHour <= 0 {
		cfg.DecayRatePerHour = 0.01
	}

	store := &HindsightStore{
		db:      db,
		cfg:     cfg,
		items:   make(map[string]*MemoryItem),
		vectors: make(map[string][]float32),
	}
	if db != nil {
		store.initTable()
		store.loadFromDB()
	}
	return store
}

func (s *HindsightStore) initTable() {
	if s.db == nil {
		return
	}
	_, _ = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS hindsight_memories (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			readership TEXT NOT NULL DEFAULT 'public',
			bank TEXT NOT NULL,
			content TEXT NOT NULL,
			tokens_estimate INTEGER NOT NULL,
			importance REAL NOT NULL,
			effective_score REAL NOT NULL,
			created_at TIMESTAMP NOT NULL,
			last_accessed_at TIMESTAMP NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_mem_bank ON hindsight_memories(bank);
		CREATE INDEX IF NOT EXISTS idx_mem_tenant ON hindsight_memories(tenant_id);
		CREATE INDEX IF NOT EXISTS idx_mem_score ON hindsight_memories(effective_score);
		CREATE TABLE IF NOT EXISTS hindsight_vectors (
			memory_id TEXT PRIMARY KEY,
			embedding BLOB NOT NULL
		);
	`)

	// Schema migrations for backward compatibility
	_, _ = s.db.Exec(`ALTER TABLE hindsight_memories ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default'`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_memories ADD COLUMN readership TEXT NOT NULL DEFAULT 'public'`)
}

func (s *HindsightStore) loadFromDB() {
	if s.db == nil {
		return
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, readership, bank, content, tokens_estimate, importance, effective_score, created_at, last_accessed_at FROM hindsight_memories`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item MemoryItem
			var bankStr string
			if err := rows.Scan(&item.ID, &item.TenantID, &item.Readership, &bankStr, &item.Content, &item.TokensEstimate, &item.Importance, &item.EffectiveScore, &item.CreatedAt, &item.LastAccessedAt); err == nil {
				item.Bank = MemoryBank(bankStr)
				s.items[item.ID] = &item
			}
		}
	}

	vecRows, err := s.db.Query(`SELECT memory_id, embedding FROM hindsight_vectors`)
	if err == nil {
		defer vecRows.Close()
		for vecRows.Next() {
			var memID string
			var blob []byte
			if err := vecRows.Scan(&memID, &blob); err == nil {
				if vec, err := DecodeEmbedding(blob); err == nil {
					s.vectors[memID] = vec
				}
			}
		}
	}
}

// Retain stores a new memory item with default tenant scoping and auto-generates 384d semantic embedding
func (s *HindsightStore) Retain(bank MemoryBank, content string, importance float64) *MemoryItem {
	return s.RetainScoped("default", "public", bank, content, importance, nil)
}

// RetainWithEmbedding stores a memory item with custom or generated embedding
func (s *HindsightStore) RetainWithEmbedding(bank MemoryBank, content string, importance float64, customEmbedding []float32) *MemoryItem {
	return s.RetainScoped("default", "public", bank, content, importance, customEmbedding)
}

// RetainScoped stores a memory item with explicit multi-tenant and IFC readership scoping
func (s *HindsightStore) RetainScoped(
	tenantID string,
	readership string,
	bank MemoryBank,
	content string,
	importance float64,
	customEmbedding []float32,
) *MemoryItem {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	item := &MemoryItem{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		Readership:     readership,
		Bank:           bank,
		Content:        content,
		TokensEstimate: tokens,
		Importance:     importance,
		EffectiveScore: importance,
		CreatedAt:      time.Now().UTC(),
		LastAccessedAt: time.Now().UTC(),
	}

	emb := customEmbedding
	if len(emb) == 0 {
		emb = GenerateSemanticEmbedding(content)
	}

	s.items[item.ID] = item
	s.vectors[item.ID] = emb

	if s.db != nil {
		_, _ = s.db.Exec(`
			INSERT INTO hindsight_memories (id, tenant_id, readership, bank, content, tokens_estimate, importance, effective_score, created_at, last_accessed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, item.ID, item.TenantID, item.Readership, string(item.Bank), item.Content, item.TokensEstimate, item.Importance, item.EffectiveScore, item.CreatedAt, item.LastAccessedAt)

		encodedBlob := EncodeEmbedding(emb)
		_, _ = s.db.Exec(`
			INSERT OR REPLACE INTO hindsight_vectors (memory_id, embedding)
			VALUES (?, ?)
		`, item.ID, encodedBlob)
	}

	// Enforce capacity bounds and LRU eviction
	s.evictIfExceededLocked(bank)

	return item
}

// evictIfExceededLocked removes low-scoring / least recently accessed memories when bank limits are exceeded
func (s *HindsightStore) evictIfExceededLocked(bank MemoryBank) int {
	var bankItems []*MemoryItem
	totalTokens := 0
	for _, item := range s.items {
		if item.Bank == bank {
			bankItems = append(bankItems, item)
			totalTokens += item.TokensEstimate
		}
	}

	if len(bankItems) <= s.cfg.MaxItemsPerBank && totalTokens <= s.cfg.MaxTokensPerBank {
		return 0
	}

	// Sort ascending by EffectiveScore (lowest importance purged first)
	// Secondary sort by LastAccessedAt (oldest accessed purged first)
	sort.Slice(bankItems, func(i, j int) bool {
		if bankItems[i].EffectiveScore == bankItems[j].EffectiveScore {
			return bankItems[i].LastAccessedAt.Before(bankItems[j].LastAccessedAt)
		}
		return bankItems[i].EffectiveScore < bankItems[j].EffectiveScore
	})

	evictedCount := 0
	for len(bankItems) > s.cfg.MaxItemsPerBank || totalTokens > s.cfg.MaxTokensPerBank {
		toPurge := bankItems[0]
		bankItems = bankItems[1:]
		totalTokens -= toPurge.TokensEstimate
		delete(s.items, toPurge.ID)
		delete(s.vectors, toPurge.ID)

		if s.db != nil {
			_, _ = s.db.Exec(`DELETE FROM hindsight_memories WHERE id = ?`, toPurge.ID)
			_, _ = s.db.Exec(`DELETE FROM hindsight_vectors WHERE memory_id = ?`, toPurge.ID)
		}
		evictedCount++
	}

	return evictedCount
}

// Recall queries memory within a hard token recall budget using keyword/substring matching
func (s *HindsightStore) Recall(bank MemoryBank, query string, maxTokens int) []*MemoryItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	queryLower := strings.ToLower(query)
	var matched []*MemoryItem

	for _, item := range s.items {
		if bank != "" && item.Bank != bank {
			continue
		}
		if queryLower == "" || strings.Contains(strings.ToLower(item.Content), queryLower) {
			matched = append(matched, item)
		}
	}

	// Sort by effective score descending
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].EffectiveScore > matched[j].EffectiveScore
	})

	// Pack items up to token budget
	var budgeted []*MemoryItem
	currentTokens := 0
	for _, item := range matched {
		if currentTokens+item.TokensEstimate > maxTokens && len(budgeted) > 0 {
			break
		}
		item.LastAccessedAt = time.Now().UTC()
		budgeted = append(budgeted, item)
		currentTokens += item.TokensEstimate
	}

	return budgeted
}

// ListRecent inventories a tenant's memories newest-first within a token
// budget. It exists because semantic search with an empty query is
// meaningless (the empty string embeds to the zero vector); callers that
// want "what is in memory" use this, callers that want "what matches"
// use recall. Listing does not touch LastAccessedAt: inventory is not use,
// and must not protect rows from LRU eviction.
func (s *HindsightStore) ListRecent(bank MemoryBank, tenant string, maxTokens int) []*MemoryItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tenant == "" {
		tenant = "default"
	}
	var matched []*MemoryItem
	for _, item := range s.items {
		if bank != "" && item.Bank != bank {
			continue
		}
		itemTenant := item.TenantID
		if itemTenant == "" {
			itemTenant = "default"
		}
		if itemTenant != tenant {
			continue
		}
		matched = append(matched, item)
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})
	var out []*MemoryItem
	currentTokens := 0
	for _, item := range matched {
		if currentTokens+item.TokensEstimate > maxTokens && len(out) > 0 {
			break
		}
		out = append(out, item)
		currentTokens += item.TokensEstimate
	}
	return out
}

// SemanticRecall queries memory using 384-dimensional cosine similarity ranking with default tenant scoping
func (s *HindsightStore) SemanticRecall(bank MemoryBank, queryText string, maxTokens int, minSimilarity float64) []*ScoredMemoryItem {
	return s.SemanticRecallScoped(bank, queryText, maxTokens, minSimilarity, "default", []string{"default"})
}

// SemanticRecallScoped enforces Buzz IFC reader-set lattice scoping and tenant isolation on semantic recall
func (s *HindsightStore) SemanticRecallScoped(
	bank MemoryBank,
	queryText string,
	maxTokens int,
	minSimilarity float64,
	requesterTenant string,
	requesterPrincipals []string,
) []*ScoredMemoryItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	queryVec := GenerateSemanticEmbedding(queryText)

	// Prepare IFC destination reader-set for requester
	var destRS ifc.ReaderSet
	if len(requesterPrincipals) == 0 {
		destRS = ifc.Public()
	} else {
		destRS = ifc.Restricted(requesterPrincipals...)
	}

	var scored []*ScoredMemoryItem

	for id, item := range s.items {
		if bank != "" && item.Bank != bank {
			continue
		}

		// 1. Enforce strict tenant isolation boundary: an item is visible
		// only inside its own lab tenant. Empty tenant normalizes to
		// 'default' on both sides so single-tenant behavior is preserved
		// without a bypass any tenant can claim.
		itemTenant := item.TenantID
		if itemTenant == "" {
			itemTenant = "default"
		}
		reqTenant := requesterTenant
		if reqTenant == "" {
			reqTenant = "default"
		}
		if itemTenant != reqTenant {
			continue // Cross-tenant context leak blocked
		}

		// 2. Enforce IFC ReaderSet lattice flow: sourceRS.CanFlowTo(destRS)
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
			continue // Access denied by IFC lattice
		}

		itemVec, hasVec := s.vectors[id]
		if !hasVec {
			continue
		}

		sim := CosineSimilarity(queryVec, itemVec)
		if sim >= minSimilarity {
			scored = append(scored, &ScoredMemoryItem{
				Item:       item,
				Similarity: sim,
			})
		}
	}

	// Sort by similarity descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Similarity > scored[j].Similarity
	})

	// Pack items up to token budget
	var budgeted []*ScoredMemoryItem
	currentTokens := 0
	for _, entry := range scored {
		if currentTokens+entry.Item.TokensEstimate > maxTokens && len(budgeted) > 0 {
			break
		}
		entry.Item.LastAccessedAt = time.Now().UTC()
		budgeted = append(budgeted, entry)
		currentTokens += entry.Item.TokensEstimate
	}

	return budgeted
}

// Decay applies time-decayed memory consolidation based on mainza-consciousness:
// E(t) = max(0.1, 1.0 - 0.01 * delta_hours)
// Updates both in-memory scores and SQLite database records
func (s *HindsightStore) Decay(elapsedHours float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	factor := math.Max(0.1, 1.0-s.cfg.DecayRatePerHour*elapsedHours)
	for _, item := range s.items {
		if item.Bank == BankExperiences {
			item.EffectiveScore = item.Importance * factor
			if s.db != nil {
				_, _ = s.db.Exec(`UPDATE hindsight_memories SET effective_score = ? WHERE id = ?`, item.EffectiveScore, item.ID)
			}
		}
	}
}

// StartDecayDaemon initiates a background worker running periodic time-decay consolidation
func (s *HindsightStore) StartDecayDaemon(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.DecayInterval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				hours := s.cfg.DecayInterval.Hours()
				s.Decay(hours)
			}
		}
	}()
}

// Reflect synthesizes high-importance experiences into a consolidated mental model
func (s *HindsightStore) Reflect(summary string) *MemoryItem {
	return s.Retain(BankMentalModels, fmt.Sprintf("Consolidated model: %s", summary), 1.0)
}
