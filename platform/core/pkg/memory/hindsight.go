package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
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
	// B10: a retracted memory is a tombstone. Its content is cleared and the
	// embedding removed; recall never returns it.
	RetractedAt   *time.Time `json:"retracted_at,omitempty"`
	RetractReason string     `json:"retract_reason,omitempty"`
	// Temporal validity: a contradiction closes the old interval instead of
	// deleting history. Recall defaults to ValidTo == nil; the superseded row
	// remains queryable by id/audit.
	ValidFrom    time.Time  `json:"valid_from,omitempty"`
	ValidTo      *time.Time `json:"valid_to,omitempty"`
	SupersededBy string     `json:"superseded_by,omitempty"`
	// Outcome calibration: explicit useful/not-useful feedback adjusts
	// importance instead of leaving it frozen at turn-shape heuristics.
	HelpfulCount int `json:"helpful_count,omitempty"`
	HarmfulCount int `json:"harmful_count,omitempty"`
}

type StoreConfig struct {
	MaxItemsPerBank  int `json:"max_items_per_bank"`
	MaxTokensPerBank int `json:"max_tokens_per_bank"`
	// MaxTotalItems and MaxTotalBytes bound the WHOLE store, not one tenant.
	// Per-bank caps are isolation; without a global cap, tenant count is an
	// unbounded memory dimension (loadFromDB loads every tenant).
	MaxTotalItems    int           `json:"max_total_items"`
	MaxTotalBytes    int64         `json:"max_total_bytes"`
	DecayInterval    time.Duration `json:"decay_interval"`
	DecayRatePerHour float64       `json:"decay_rate_per_hour"`
	TombstoneTTLDays int           `json:"tombstone_ttl_days"`
}

func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		MaxItemsPerBank:  500,
		MaxTokensPerBank: 250000,
		MaxTotalItems:    200000,
		MaxTotalBytes:    300 << 20,
		DecayInterval:    1 * time.Hour,
		DecayRatePerHour: 0.01, // 1% decay per hour
		TombstoneTTLDays: 30,
	}
}

// StoreStats is the observable shape of the live store. It is deliberately
// cheap (map lengths + one pass for vector bytes) so /metrics can call it on
// every scrape without perturbing recall latency.
type StoreStats struct {
	Items       int   `json:"items"`
	Vectors     int   `json:"vectors"`
	Retracted   int   `json:"retracted"`
	VectorBytes int64 `json:"vector_bytes"`
	TotalBytes  int64 `json:"total_bytes"`
}

type cachedEmbedding struct {
	vec   []float32
	model string
}

// embedFor resolves an embedding with a bounded content cache. The HTTP
// call happens WITHOUT the store lock: one slow endpoint must not freeze
// every tenant's recall/retain.
func (s *HindsightStore) embedFor(text string) ([]float32, string) {
	key := sha256.Sum256([]byte(text))
	s.cacheMu.Lock()
	if cached, ok := s.embedCache[key]; ok {
		s.cacheMu.Unlock()
		return cached.vec, cached.model
	}
	s.cacheMu.Unlock()

	s.mu.RLock()
	embedder := s.embedder
	s.mu.RUnlock()
	if embedder != nil {
		if vec, model, err := embedder.Embed(context.Background(), text); err == nil && len(vec) > 0 {
			s.rememberEmbedding(key, vec, model)
			return vec, model
		}
	}
	return GenerateSemanticEmbedding(text), LexicalEmbeddingModel
}

func (s *HindsightStore) rememberEmbedding(key [32]byte, vec []float32, model string) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if _, exists := s.embedCache[key]; exists {
		return
	}
	const maxCached = 256
	for len(s.embedOrder) >= maxCached {
		oldest := s.embedOrder[0]
		s.embedOrder = s.embedOrder[1:]
		delete(s.embedCache, oldest)
	}
	s.embedCache[key] = cachedEmbedding{vec: append([]float32(nil), vec...), model: model}
	s.embedOrder = append(s.embedOrder, key)
}

// Stats reports resident cardinality and embedding footprint.
func (s *HindsightStore) Stats() StoreStats {
	if s == nil {
		return StoreStats{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := StoreStats{Items: len(s.items), Vectors: len(s.vectors)}
	for _, item := range s.items {
		if item.RetractedAt != nil {
			st.Retracted++
		}
	}
	for id, vec := range s.vectors {
		st.VectorBytes += int64(len(vec))
		if item := s.items[id]; item != nil {
			st.TotalBytes += int64(len(item.Content)+len(vec)) + 128
		}
	}
	return st
}

type HindsightStore struct {
	mu           sync.RWMutex
	db           *sql.DB
	cfg          StoreConfig
	items        map[string]*MemoryItem
	vectors      map[string][]int8
	vectorScales map[string]float32
	vectorModels map[string]string
	embedder     TextEmbedder
	// embedCache is a bounded single-flight-by-content cache. Embedding
	// endpoints are the slowest part of retain/recall; repeated prompts
	// should not pay a network round trip under the store lock.
	embedCache map[[32]byte]cachedEmbedding
	embedOrder [][32]byte
	cacheMu    sync.Mutex
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
	if cfg.MaxTotalItems <= 0 {
		cfg.MaxTotalItems = 200000
	}
	if cfg.MaxTotalBytes <= 0 {
		cfg.MaxTotalBytes = 300 << 20
	}
	if cfg.DecayInterval <= 0 {
		cfg.DecayInterval = 1 * time.Hour
	}
	if cfg.DecayRatePerHour <= 0 {
		cfg.DecayRatePerHour = 0.01
	}
	if cfg.TombstoneTTLDays <= 0 {
		cfg.TombstoneTTLDays = 30
	}

	store := &HindsightStore{
		db:           db,
		cfg:          cfg,
		items:        make(map[string]*MemoryItem),
		vectors:      make(map[string][]int8),
		vectorScales: make(map[string]float32),
		vectorModels: make(map[string]string),
		embedder:     NewEnvEmbedder(),
		embedCache:   map[[32]byte]cachedEmbedding{},
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
			last_accessed_at TIMESTAMP NOT NULL,
			retracted_at TIMESTAMP,
			retract_reason TEXT NOT NULL DEFAULT '',
			valid_from TIMESTAMP,
			valid_to TIMESTAMP,
			superseded_by TEXT NOT NULL DEFAULT '',
			helpful_count INTEGER NOT NULL DEFAULT 0,
			harmful_count INTEGER NOT NULL DEFAULT 0,
			embedding_int8 BLOB,
			embedding_scale REAL NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_mem_bank ON hindsight_memories(bank);
		CREATE INDEX IF NOT EXISTS idx_mem_tenant ON hindsight_memories(tenant_id);
		CREATE INDEX IF NOT EXISTS idx_mem_score ON hindsight_memories(effective_score);
		CREATE TABLE IF NOT EXISTS hindsight_vectors (
			memory_id TEXT PRIMARY KEY,
			embedding BLOB NOT NULL,
			embedding_int8 BLOB,
			embedding_scale REAL NOT NULL DEFAULT 0,
			embedding_model TEXT NOT NULL DEFAULT 'lexical-hash-v1',
			embedding_dim INTEGER NOT NULL DEFAULT 384
		);
	`)

	// Schema migrations for backward compatibility
	_, _ = s.db.Exec(`ALTER TABLE hindsight_memories ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default'`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_memories ADD COLUMN readership TEXT NOT NULL DEFAULT 'public'`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_memories ADD COLUMN retracted_at TIMESTAMP`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_memories ADD COLUMN retract_reason TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_vectors ADD COLUMN embedding_model TEXT NOT NULL DEFAULT 'lexical-hash-v1'`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_vectors ADD COLUMN embedding_dim INTEGER NOT NULL DEFAULT 384`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_vectors ADD COLUMN embedding_int8 BLOB`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_vectors ADD COLUMN embedding_scale REAL NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_memories ADD COLUMN valid_from TIMESTAMP`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_memories ADD COLUMN valid_to TIMESTAMP`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_memories ADD COLUMN superseded_by TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_memories ADD COLUMN helpful_count INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_memories ADD COLUMN harmful_count INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_memories ADD COLUMN embedding_int8 BLOB`)
	_, _ = s.db.Exec(`ALTER TABLE hindsight_memories ADD COLUMN embedding_scale REAL NOT NULL DEFAULT 0`)
}

func (s *HindsightStore) loadFromDB() {
	if s.db == nil {
		return
	}
	limit := s.cfg.MaxTotalItems
	if limit <= 0 {
		limit = 1000
	}
	// Single LEFT JOIN guarantees loaded memories and vectors match one snapshot.
	rows, err := s.db.Query(`SELECT m.id, m.tenant_id, m.readership, m.bank, m.content,
		m.tokens_estimate, m.importance, m.effective_score, m.created_at, m.last_accessed_at,
		m.retracted_at, m.retract_reason, m.valid_from, m.valid_to, m.superseded_by,
		m.helpful_count, m.harmful_count,
		v.embedding, v.embedding_int8, COALESCE(v.embedding_scale, 0), COALESCE(v.embedding_model, 'lexical-hash-v1')
		FROM hindsight_memories m
		LEFT JOIN hindsight_vectors v ON v.memory_id = m.id
		ORDER BY m.effective_score DESC LIMIT ?`, limit)
	if err == nil {
		defer rows.Close()
		type backfill struct {
			id    string
			q     []int8
			scale float32
		}
		var toBackfill []backfill
		for rows.Next() {
			var item MemoryItem
			var bankStr, model string
			var retracted, validFrom, validTo sql.NullTime
			var superseded sql.NullString
			var f32blob, i8blob []byte
			var scale float64
			if err := rows.Scan(&item.ID, &item.TenantID, &item.Readership, &bankStr, &item.Content,
				&item.TokensEstimate, &item.Importance, &item.EffectiveScore, &item.CreatedAt, &item.LastAccessedAt,
				&retracted, &item.RetractReason, &validFrom, &validTo, &superseded,
				&item.HelpfulCount, &item.HarmfulCount,
				&f32blob, &i8blob, &scale, &model); err == nil {
				item.Bank = MemoryBank(bankStr)
				if retracted.Valid {
					t := retracted.Time
					item.RetractedAt = &t
				}
				if validFrom.Valid {
					item.ValidFrom = validFrom.Time
				}
				if validTo.Valid {
					t := validTo.Time
					item.ValidTo = &t
				}
				item.SupersededBy = superseded.String
				s.items[item.ID] = &item
				if len(i8blob) > 0 {
					s.vectors[item.ID] = DecodeInt8(i8blob)
					s.vectorScales[item.ID] = float32(scale)
					s.vectorModels[item.ID] = model
				} else if len(f32blob) > 0 {
					if vec, derr := DecodeEmbedding(f32blob); derr == nil {
						q, qscale := QuantizeEmbedding(vec)
						s.vectors[item.ID] = q
						s.vectorScales[item.ID] = qscale
						s.vectorModels[item.ID] = model
						toBackfill = append(toBackfill, backfill{id: item.ID, q: q, scale: qscale})
					}
				}
			}
		}
		rows.Close()
		for _, bf := range toBackfill {
			_, _ = s.db.Exec(`UPDATE hindsight_vectors SET embedding_int8 = ?, embedding_scale = ? WHERE memory_id = ?`,
				EncodeInt8(bf.q), bf.scale, bf.id)
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
	if tenantID == "" {
		tenantID = "default"
	}
	if readership == "" {
		readership = "public"
	}
	if importance < 0 {
		importance = 0
	}
	if importance > 1 {
		importance = 1
	}

	// Embedding happens before the lock (F4): one slow endpoint must not
	// block every other tenant's retain/recall.
	emb := customEmbedding
	embModel := "custom"
	if len(emb) == 0 {
		emb, embModel = s.embedFor(content)
	}

	tokens := len(content) / 4
	if tokens < 1 {
		tokens = 1
	}
	now := time.Now().UTC()
	embQ, _ := QuantizeEmbedding(emb)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Dedupe/merge: a repeated turn must not consume the bank cap and evict
	// useful history. Exact content is matched cheaply; near-duplicates use
	// the embedding already computed above.
	for _, existing := range s.items {
		if existing.RetractedAt != nil || existing.ValidTo != nil || existing.Bank != bank ||
			existing.TenantID != tenantID || existing.Readership != readership {
			continue
		}
		duplicate := existing.Content == content
		if !duplicate && len(embQ) > 0 && len(s.vectors[existing.ID]) == len(embQ) && s.vectorModels[existing.ID] == embModel {
			duplicate = CosineSimilarityInt8(embQ, s.vectors[existing.ID]) >= 0.97
		}
		if !duplicate {
			continue
		}
		if importance > existing.Importance {
			existing.Importance = importance
			existing.EffectiveScore = importance
		}
		existing.LastAccessedAt = now
		if s.db != nil {
			_, _ = s.db.Exec(`UPDATE hindsight_memories SET importance = ?, effective_score = ?, last_accessed_at = ? WHERE id = ?`,
				existing.Importance, existing.EffectiveScore, existing.LastAccessedAt, existing.ID)
		}
		return existing
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
		CreatedAt:      now,
		LastAccessedAt: now,
		ValidFrom:      now,
	}

	itemQ, itemScale := QuantizeEmbedding(emb)
	s.items[item.ID] = item
	s.vectors[item.ID] = itemQ
	s.vectorScales[item.ID] = itemScale
	s.vectorModels[item.ID] = embModel

	if s.db != nil {
		_, _ = s.db.Exec(`
			INSERT INTO hindsight_memories (id, tenant_id, readership, bank, content, tokens_estimate, importance, effective_score, created_at, last_accessed_at, valid_from)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, item.ID, item.TenantID, item.Readership, string(item.Bank), item.Content, item.TokensEstimate, item.Importance, item.EffectiveScore, item.CreatedAt, item.LastAccessedAt, item.ValidFrom)

		encodedBlob := EncodeEmbedding(emb)
		_, _ = s.db.Exec(`
			INSERT OR REPLACE INTO hindsight_vectors (memory_id, embedding, embedding_int8, embedding_scale, embedding_model, embedding_dim)
			VALUES (?, ?, ?, ?, ?, ?)
		`, item.ID, encodedBlob, EncodeInt8(itemQ), itemScale, embModel, len(emb))
	}

	// Enforce capacity bounds and LRU eviction within the SAME tenant +
	// readership: one lab filling its bank must never evict another lab's
	// memories (availability isolation).
	s.evictIfExceededLocked(bank, tenantID, readership)
	s.enforceGlobalBudgetLocked()
	return item
}

// enforceGlobalBudgetLocked is the last-resort whole-store bound. It prefers
// evicting from tenants over their fair share, then the lowest-scoring oldest
// item, so one runaway lab cannot silently delete every other lab's memory.
func (s *HindsightStore) enforceGlobalBudgetLocked() int {
	if s.cfg.MaxTotalItems <= 0 && s.cfg.MaxTotalBytes <= 0 {
		return 0
	}
	totalBytes := s.totalBytesLocked()
	if len(s.items) <= s.cfg.MaxTotalItems && totalBytes <= s.cfg.MaxTotalBytes {
		return 0
	}
	perTenant := 1
	if n := s.tenantCountLocked(); n > 0 {
		perTenant = s.cfg.MaxTotalItems / n
		if perTenant < 1 {
			perTenant = 1
		}
	}
	counts := map[string]int{}
	for _, item := range s.items {
		counts[item.TenantID]++
	}
	type candidate struct {
		item *MemoryItem
		over bool
	}
	cands := make([]candidate, 0, len(s.items))
	for _, item := range s.items {
		cands = append(cands, candidate{item: item, over: counts[item.TenantID] > perTenant})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].over != cands[j].over {
			return cands[i].over
		}
		if cands[i].item.EffectiveScore != cands[j].item.EffectiveScore {
			return cands[i].item.EffectiveScore < cands[j].item.EffectiveScore
		}
		return cands[i].item.LastAccessedAt.Before(cands[j].item.LastAccessedAt)
	})
	evicted := 0
	for _, c := range cands {
		if len(s.items) <= s.cfg.MaxTotalItems && totalBytes <= s.cfg.MaxTotalBytes {
			break
		}
		itemBytes := int64(len(c.item.Content)) + 128 + int64(len(s.vectors[c.item.ID]))
		totalBytes -= itemBytes
		delete(s.items, c.item.ID)
		delete(s.vectors, c.item.ID)
		delete(s.vectorScales, c.item.ID)
		delete(s.vectorModels, c.item.ID)
		if s.db != nil {
			_, _ = s.db.Exec(`DELETE FROM hindsight_memories WHERE id = ?`, c.item.ID)
			_, _ = s.db.Exec(`DELETE FROM hindsight_vectors WHERE memory_id = ?`, c.item.ID)
		}
		evicted++
	}
	return evicted
}

func (s *HindsightStore) tenantCountLocked() int {
	seen := map[string]struct{}{}
	for _, item := range s.items {
		seen[item.TenantID] = struct{}{}
	}
	return len(seen)
}

func (s *HindsightStore) totalBytesLocked() int64 {
	var total int64
	for id, item := range s.items {
		total += int64(len(item.Content)) + 128
		if vec := s.vectors[id]; len(vec) > 0 {
			total += int64(len(vec))
		}
	}
	return total
}

// PurgeRetracted removes tombstones older than before from memory and disk.
// Retraction is terminal, but a tombstone need not be resident forever.
func (s *HindsightStore) PurgeRetracted(before time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for id, item := range s.items {
		if item.RetractedAt == nil || !item.RetractedAt.Before(before) {
			continue
		}
		delete(s.items, id)
		delete(s.vectors, id)
		delete(s.vectorScales, id)
		delete(s.vectorModels, id)
		if s.db != nil {
			_, _ = s.db.Exec(`DELETE FROM hindsight_memories WHERE id = ?`, id)
			_, _ = s.db.Exec(`DELETE FROM hindsight_vectors WHERE memory_id = ?`, id)
		}
		removed++
	}
	return removed
}

// evictIfExceededLocked removes low-scoring / least recently accessed memories when bank limits are exceeded
func (s *HindsightStore) evictIfExceededLocked(bank MemoryBank, tenantID, readership string) int {
	var bankItems []*MemoryItem
	totalTokens := 0
	for _, item := range s.items {
		if item.Bank == bank && item.TenantID == tenantID && item.Readership == readership {
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
		delete(s.vectorScales, toPurge.ID)
		delete(s.vectorModels, toPurge.ID)

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
		if item.RetractedAt != nil || item.ValidTo != nil {
			continue // tombstone or superseded: never recalled
		}
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
		if item.RetractedAt != nil || item.ValidTo != nil {
			continue // tombstone or superseded: never listed
		}
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
	// Embed before taking the store lock (F4). A slow endpoint delays this
	// query, not every other tenant's memory operations.
	queryVec, queryModel := s.embedFor(queryText)
	lexicalQueryVec := GenerateSemanticEmbedding(queryText)
	queryInt8, _ := QuantizeEmbedding(queryVec)
	lexicalInt8, _ := QuantizeEmbedding(lexicalQueryVec)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Prepare IFC destination reader-set for requester
	var destRS ifc.ReaderSet
	if len(requesterPrincipals) == 0 {
		destRS = ifc.Public()
	} else {
		destRS = ifc.Restricted(requesterPrincipals...)
	}

	var scored []*ScoredMemoryItem

	for id, item := range s.items {
		if item.RetractedAt != nil || item.ValidTo != nil {
			continue // tombstone or superseded: never recalled
		}
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

		qv := queryInt8
		itemModel := s.vectorModels[id]
		if (len(qv) != len(itemVec) || (itemModel != "" && queryModel != "" && itemModel != queryModel && itemModel != "custom")) &&
			len(lexicalInt8) == len(itemVec) && (itemModel == "" || itemModel == LexicalEmbeddingModel) {
			qv = lexicalInt8
		}
		if itemModel != "" && queryModel != "" && itemModel != queryModel && itemModel != "custom" && itemModel != LexicalEmbeddingModel {
			continue
		}
		sim := CosineSimilarityInt8(qv, itemVec)
		if sim >= minSimilarity {
			// Hybrid rank: semantic similarity dominates, lexical overlap
			// catches exact identifiers the embedding misses, and a small
			// recency prior keeps two equally relevant memories ordered.
			rank := 0.70*sim + 0.15*lexicalOverlap(queryText, item.Content) + 0.10*recencyWeight(item.LastAccessedAt) + 0.05*item.EffectiveScore
			scored = append(scored, &ScoredMemoryItem{
				Item:       item,
				Similarity: sim,
				Rank:       rank,
			})
		}
	}

	// Sort by the hybrid rank descending; Similarity stays the raw signal.
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Rank > scored[j].Rank
	})

	// Pack items up to token budget, returning detached copies so callers
	// never race on internal MemoryItem pointers.
	var budgeted []*ScoredMemoryItem
	currentTokens := 0
	for _, entry := range scored {
		if currentTokens+entry.Item.TokensEstimate > maxTokens && len(budgeted) > 0 {
			break
		}
		entry.Item.LastAccessedAt = time.Now().UTC()
		cp := *entry.Item
		budgeted = append(budgeted, &ScoredMemoryItem{
			Item:       &cp,
			Similarity: entry.Similarity,
			Rank:       entry.Rank,
		})
		currentTokens += entry.Item.TokensEstimate
	}

	return budgeted
}

func lexicalOverlap(query, content string) float64 {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return 0
	}
	text := strings.ToLower(content)
	seen := map[string]struct{}{}
	hits := 0
	for _, term := range terms {
		if len(term) < 3 {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		if strings.Contains(text, term) {
			hits++
		}
	}
	if len(seen) == 0 {
		return 0
	}
	return float64(hits) / float64(len(seen))
}

func recencyWeight(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	days := time.Since(t).Hours() / 24
	if days < 0 {
		days = 0
	}
	return math.Exp(-days / 30.0)
}

// Decay applies time-decayed memory consolidation based on mainza-consciousness:
// E(t) = max(0.1, 1.0 - 0.01 * delta_hours)
// Updates both in-memory scores and SQLite database records
// ErrMemoryNotFound is returned when a retraction names a row outside the
// caller's tenant (or no row at all) — no existence oracle.
var ErrMemoryNotFound = errors.New("memory not found")

// RetractScoped tombstones one memory: the content is cleared, the embedding
// removed, and the row kept for audit. Recall never returns a tombstone.
func (s *HindsightStore) RetractScoped(tenantID, id, reason string) error {
	if tenantID == "" {
		tenantID = "default"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return ErrMemoryNotFound
	}
	itemTenant := item.TenantID
	if itemTenant == "" {
		itemTenant = "default"
	}
	if itemTenant != tenantID {
		return ErrMemoryNotFound
	}
	now := time.Now().UTC()
	item.RetractedAt = &now
	item.RetractReason = reason
	item.Content = "[retracted]"
	item.TokensEstimate = 1
	delete(s.vectors, id)
	delete(s.vectorScales, id)
	delete(s.vectorModels, id)
	if s.db != nil {
		_, _ = s.db.Exec(`UPDATE hindsight_memories SET content = ?, tokens_estimate = 1, retracted_at = ?, retract_reason = ? WHERE id = ?`,
			item.Content, now, reason, id)
		_, _ = s.db.Exec(`DELETE FROM hindsight_vectors WHERE memory_id = ?`, id)
	}
	return nil
}

// SupersedeScoped closes an old memory's validity interval. History is kept
// (the row remains queryable by id/audit); recall defaults to current facts.
func (s *HindsightStore) SupersedeScoped(tenantID, oldID, newID string) error {
	if tenantID == "" {
		tenantID = "default"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[oldID]
	if !ok {
		return ErrMemoryNotFound
	}
	itemTenant := item.TenantID
	if itemTenant == "" {
		itemTenant = "default"
	}
	if itemTenant != tenantID {
		return ErrMemoryNotFound
	}
	now := time.Now().UTC()
	item.ValidTo = &now
	item.SupersededBy = newID
	if s.db != nil {
		_, _ = s.db.Exec(`UPDATE hindsight_memories SET valid_to = ?, superseded_by = ? WHERE id = ?`, now, newID, oldID)
	}
	return nil
}

// FeedbackScoped records an explicit outcome for one memory and recalibrates
// importance. The beta(1,1) prior keeps a single vote from swinging the score;
// the 50/50 blend preserves the original heuristic signal.
func (s *HindsightStore) FeedbackScoped(tenantID, id string, helpful bool) error {
	if tenantID == "" {
		tenantID = "default"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return ErrMemoryNotFound
	}
	itemTenant := item.TenantID
	if itemTenant == "" {
		itemTenant = "default"
	}
	if itemTenant != tenantID {
		return ErrMemoryNotFound
	}
	if helpful {
		item.HelpfulCount++
	} else {
		item.HarmfulCount++
	}
	posterior := float64(item.HelpfulCount+1) / float64(item.HelpfulCount+item.HarmfulCount+2)
	item.Importance = 0.5*item.Importance + 0.5*posterior
	if item.Importance > 1 {
		item.Importance = 1
	}
	item.EffectiveScore = item.Importance
	if s.db != nil {
		_, _ = s.db.Exec(`UPDATE hindsight_memories SET importance = ?, effective_score = ?, helpful_count = ?, harmful_count = ? WHERE id = ?`,
			item.Importance, item.EffectiveScore, item.HelpfulCount, item.HarmfulCount, item.ID)
	}
	return nil
}

// TemporalNode/Edge are the memory supersession projection: history is kept,
// current facts are the nodes with no ValidTo.
type TemporalNode struct {
	ID           string     `json:"id"`
	Bank         string     `json:"bank"`
	Content      string     `json:"content"`
	ValidFrom    time.Time  `json:"valid_from,omitempty"`
	ValidTo      *time.Time `json:"valid_to,omitempty"`
	SupersededBy string     `json:"superseded_by,omitempty"`
}

type TemporalEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type TemporalGraph struct {
	Tenant string         `json:"tenant"`
	Nodes  []TemporalNode `json:"nodes"`
	Edges  []TemporalEdge `json:"edges"`
}

// TemporalGraph projects a tenant's memories and supersession links from the
// ledger. It is tenant-scoped and read-only; the console does not consume it
// yet, so it is exposed through the operator CLI, not a new HTTP contract.
func (s *HindsightStore) TemporalGraph(tenantID string) (TemporalGraph, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	out := TemporalGraph{Tenant: tenantID, Nodes: []TemporalNode{}, Edges: []TemporalEdge{}}
	if s.db == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, item := range s.items {
			if item.TenantID != tenantID {
				continue
			}
			out.Nodes = append(out.Nodes, TemporalNode{ID: item.ID, Bank: string(item.Bank), Content: item.Content, ValidFrom: item.ValidFrom, ValidTo: item.ValidTo, SupersededBy: item.SupersededBy})
			if item.SupersededBy != "" {
				out.Edges = append(out.Edges, TemporalEdge{From: item.ID, To: item.SupersededBy, Kind: "supersedes"})
			}
		}
		return out, nil
	}
	rows, err := s.db.Query(`SELECT id, bank, content, valid_from, valid_to, superseded_by FROM hindsight_memories WHERE tenant_id = ? ORDER BY created_at`, tenantID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var node TemporalNode
		var validFrom, validTo sql.NullTime
		var superseded sql.NullString
		if err := rows.Scan(&node.ID, &node.Bank, &node.Content, &validFrom, &validTo, &superseded); err != nil {
			return out, err
		}
		if validFrom.Valid {
			node.ValidFrom = validFrom.Time
		}
		if validTo.Valid {
			t := validTo.Time
			node.ValidTo = &t
		}
		node.SupersededBy = superseded.String
		out.Nodes = append(out.Nodes, node)
		if node.SupersededBy != "" {
			out.Edges = append(out.Edges, TemporalEdge{From: node.ID, To: node.SupersededBy, Kind: "supersedes"})
		}
	}
	return out, rows.Err()
}

// SetEmbedder configures the neural TextEmbedder used for Retain and Recall.
func (s *HindsightStore) SetEmbedder(e TextEmbedder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embedder = e
	s.cacheMu.Lock()
	s.embedCache = map[[32]byte]cachedEmbedding{}
	s.embedOrder = nil
	s.cacheMu.Unlock()
}

func (s *HindsightStore) Decay(elapsedHours float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	type scoreUpdate struct {
		id    string
		score float64
	}
	var updates []scoreUpdate
	for _, item := range s.items {
		if item.Bank == BankExperiences {
			effectiveHours := elapsedHours
			if !item.CreatedAt.IsZero() {
				if age := now.Sub(item.CreatedAt).Hours(); age > effectiveHours {
					effectiveHours = age
				}
			}
			factor := math.Max(0.1, 1.0-s.cfg.DecayRatePerHour*effectiveHours)
			item.EffectiveScore = item.Importance * factor
			if s.db != nil {
				updates = append(updates, scoreUpdate{id: item.ID, score: item.EffectiveScore})
			}
		}
	}
	if s.db != nil && len(updates) > 0 {
		if tx, err := s.db.Begin(); err == nil {
			stmt, serr := tx.Prepare(`UPDATE hindsight_memories SET effective_score = ? WHERE id = ?`)
			if serr == nil {
				for _, u := range updates {
					_, _ = stmt.Exec(u.score, u.id)
				}
				_ = stmt.Close()
				_ = tx.Commit()
			} else {
				_ = tx.Rollback()
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
				s.PurgeRetracted(time.Now().UTC().AddDate(0, 0, -s.cfg.TombstoneTTLDays))
			}
		}
	}()
}

// Reflect synthesizes high-importance experiences into a consolidated mental model
func (s *HindsightStore) Reflect(summary string) *MemoryItem {
	return s.Retain(BankMentalModels, fmt.Sprintf("Consolidated model: %s", summary), 1.0)
}
