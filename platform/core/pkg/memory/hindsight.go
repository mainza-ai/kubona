package memory

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type MemoryBank string

const (
	BankWorldFacts   MemoryBank = "world-facts"
	BankExperiences  MemoryBank = "experiences"
	BankMentalModels MemoryBank = "mental-models"
)

type MemoryItem struct {
	ID             string     `json:"id"`
	Bank           MemoryBank `json:"bank"`
	Content        string     `json:"content"`
	TokensEstimate int        `json:"tokens_estimate"`
	Importance     float64    `json:"importance"` // 0.0 to 1.0
	EffectiveScore float64    `json:"effective_score"`
	CreatedAt      time.Time  `json:"created_at"`
	LastAccessedAt time.Time  `json:"last_accessed_at"`
}

type HindsightStore struct {
	mu    sync.RWMutex
	db    *sql.DB
	items map[string]*MemoryItem
}

func NewHindsightStore() *HindsightStore {
	return &HindsightStore{
		items: make(map[string]*MemoryItem),
	}
}

func NewPersistentHindsightStore(db *sql.DB) *HindsightStore {
	store := &HindsightStore{
		db:    db,
		items: make(map[string]*MemoryItem),
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
			bank TEXT NOT NULL,
			content TEXT NOT NULL,
			tokens_estimate INTEGER NOT NULL,
			importance REAL NOT NULL,
			effective_score REAL NOT NULL,
			created_at TIMESTAMP NOT NULL,
			last_accessed_at TIMESTAMP NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_mem_bank ON hindsight_memories(bank);
	`)
}

func (s *HindsightStore) loadFromDB() {
	if s.db == nil {
		return
	}
	rows, err := s.db.Query(`SELECT id, bank, content, tokens_estimate, importance, effective_score, created_at, last_accessed_at FROM hindsight_memories`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var item MemoryItem
		var bankStr string
		if err := rows.Scan(&item.ID, &bankStr, &item.Content, &item.TokensEstimate, &item.Importance, &item.EffectiveScore, &item.CreatedAt, &item.LastAccessedAt); err == nil {
			item.Bank = MemoryBank(bankStr)
			s.items[item.ID] = &item
		}
	}
}

// Retain stores a new memory item in the specified bank
func (s *HindsightStore) Retain(bank MemoryBank, content string, importance float64) *MemoryItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Rough token estimate: ~4 chars per token
	tokens := len(content) / 4
	if tokens < 1 {
		tokens = 1
	}

	item := &MemoryItem{
		ID:             uuid.New().String(),
		Bank:           bank,
		Content:        content,
		TokensEstimate: tokens,
		Importance:     importance,
		EffectiveScore: importance,
		CreatedAt:      time.Now().UTC(),
		LastAccessedAt: time.Now().UTC(),
	}

	s.items[item.ID] = item

	if s.db != nil {
		_, _ = s.db.Exec(`
			INSERT INTO hindsight_memories (id, bank, content, tokens_estimate, importance, effective_score, created_at, last_accessed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, item.ID, string(item.Bank), item.Content, item.TokensEstimate, item.Importance, item.EffectiveScore, item.CreatedAt, item.LastAccessedAt)
	}

	return item
}

// Recall queries memory within a hard token recall budget
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

	// Sort by effective score descending (simple bubble/insertion since matched is small)
	for i := 0; i < len(matched); i++ {
		for j := i + 1; j < len(matched); j++ {
			if matched[j].EffectiveScore > matched[i].EffectiveScore {
				matched[i], matched[j] = matched[j], matched[i]
			}
		}
	}

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

// Decay applies time-decayed memory consolidation based on mainza-consciousness:
// E(t) = max(0.1, 1.0 - 0.01 * delta_hours)
func (s *HindsightStore) Decay(elapsedHours float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	factor := math.Max(0.1, 1.0-0.01*elapsedHours)
	for _, item := range s.items {
		// World facts don't decay; experiences decay; mental models consolidate
		if item.Bank == BankExperiences {
			item.EffectiveScore = item.Importance * factor
		}
	}
}

// Reflect synthesizes high-importance experiences into a consolidated mental model
func (s *HindsightStore) Reflect(summary string) *MemoryItem {
	return s.Retain(BankMentalModels, fmt.Sprintf("Consolidated model: %s", summary), 1.0)
}
