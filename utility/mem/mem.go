package mem

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

const (
	// maxHistoryTokens is the token budget for the rolling message history.
	// When exceeded, compact is triggered.
	// DeepSeek-V3 supports 64K context; we budget ~8K for history,
	// leaving room for system prompt (~500), RAG docs (~1K), query (~200), and response (~2K).
	maxHistoryTokens = 8000
	// compactKeepTurns keeps this many most-recent conversation turns verbatim.
	compactKeepTurns = 4
)

// SummarizeFunc condenses conversation messages into a concise summary string.
// Register one at startup via SetSummarizer; without it compact falls back to dropping.
type SummarizeFunc func(ctx context.Context, msgs []*schema.Message) (string, error)

var (
	globalSummarizer SummarizeFunc
	summarizerMu     sync.RWMutex
)

// SetSummarizer registers the LLM-backed summarization function.
// Call once at server startup before handling requests.
func SetSummarizer(fn SummarizeFunc) {
	summarizerMu.Lock()
	defer summarizerMu.Unlock()
	globalSummarizer = fn
}

// PersistentMemory manages a session's conversation history in SQLite with token-based
// windowing and LLM-assisted compaction.
type PersistentMemory struct {
	sessionID string
	mu        sync.Mutex
}

// GetPersistentMemory returns a PersistentMemory for the given session ID, creating the
// SQLite session record if it does not yet exist.
func GetPersistentMemory(ctx context.Context, id string) (*PersistentMemory, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}
	var sess SessionRecord
	if db.First(&sess, "id = ?", id).Error != nil {
		sess = SessionRecord{ID: id}
		if err := db.Create(&sess).Error; err != nil {
			return nil, fmt.Errorf("mem: create session %s: %w", id, err)
		}
	}
	return &PersistentMemory{sessionID: id}, nil
}

// GetHistory returns the messages to inject as conversation history into the agent.
// If a compact summary exists it is prepended as a SystemMessage.
func (m *PersistentMemory) GetHistory(ctx context.Context) ([]*schema.Message, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	var sess SessionRecord
	if err := db.First(&sess, "id = ?", m.sessionID).Error; err != nil {
		return nil, fmt.Errorf("mem: load session: %w", err)
	}

	var records []MessageRecord
	if err := db.Where("session_id = ?", m.sessionID).Order("id asc").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("mem: load messages: %w", err)
	}

	var msgs []*schema.Message
	if sess.Summary != "" {
		msgs = append(msgs, schema.SystemMessage(
			fmt.Sprintf("以下是历史对话摘要（较早的对话内容已压缩）：\n%s", sess.Summary),
		))
	}
	for _, r := range records {
		var role schema.RoleType
		switch r.Role {
		case "user":
			role = schema.User
		case "assistant":
			role = schema.Assistant
		default:
			role = schema.System
		}
		msgs = append(msgs, &schema.Message{Role: role, Content: r.Content})
	}
	return msgs, nil
}

// AddTurn persists a user+assistant message pair and triggers compaction when the total
// history token count exceeds maxHistoryTokens.
func (m *PersistentMemory) AddTurn(ctx context.Context, userContent, assistantContent string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	db, err := getDB()
	if err != nil {
		return err
	}

	now := time.Now()
	records := []MessageRecord{
		{SessionID: m.sessionID, Role: "user", Content: userContent, Tokens: countTokens(userContent), CreatedAt: now},
		{SessionID: m.sessionID, Role: "assistant", Content: assistantContent, Tokens: countTokens(assistantContent), CreatedAt: now},
	}
	if err := db.Create(&records).Error; err != nil {
		return fmt.Errorf("mem: save turn: %w", err)
	}
	return m.maybeCompact(ctx, db)
}

// maybeCompact runs compaction if history tokens exceed the budget.
// Must be called with m.mu held.
func (m *PersistentMemory) maybeCompact(ctx context.Context, db *gorm.DB) error {
	var totalTokens int
	if err := db.Model(&MessageRecord{}).
		Where("session_id = ?", m.sessionID).
		Select("COALESCE(SUM(tokens), 0)").
		Scan(&totalTokens).Error; err != nil {
		return fmt.Errorf("mem: sum tokens: %w", err)
	}

	if totalTokens <= maxHistoryTokens {
		return nil
	}

	summarizerMu.RLock()
	summarizer := globalSummarizer
	summarizerMu.RUnlock()

	if summarizer == nil {
		return m.dropOldestPair(db)
	}
	return m.compactWithSummary(ctx, db, summarizer)
}

// compactWithSummary summarizes the oldest messages and replaces them with a summary
// stored on the session record.
func (m *PersistentMemory) compactWithSummary(ctx context.Context, db *gorm.DB, summarizer SummarizeFunc) error {
	var allRecords []MessageRecord
	if err := db.Where("session_id = ?", m.sessionID).Order("id asc").Find(&allRecords).Error; err != nil {
		return err
	}

	keepCount := compactKeepTurns * 2
	if len(allRecords) <= keepCount {
		return nil
	}
	toCompact := allRecords[:len(allRecords)-keepCount]

	var sess SessionRecord
	if err := db.First(&sess, "id = ?", m.sessionID).Error; err != nil {
		return err
	}

	// Messages to summarize: existing summary (if any) + messages being compacted
	var compactMsgs []*schema.Message
	if sess.Summary != "" {
		compactMsgs = append(compactMsgs, schema.UserMessage(
			fmt.Sprintf("这是之前对话的摘要：%s", sess.Summary),
		))
	}
	for _, r := range toCompact {
		role := schema.RoleType(schema.User)
		if r.Role == "assistant" {
			role = schema.Assistant
		}
		compactMsgs = append(compactMsgs, &schema.Message{Role: role, Content: r.Content})
	}

	summary, err := summarizer(ctx, compactMsgs)
	if err != nil {
		// Summarizer failed: fall back to dropping
		return m.dropOldestPair(db)
	}

	// Delete the compacted records
	ids := make([]uint, len(toCompact))
	for i, r := range toCompact {
		ids[i] = r.ID
	}
	if err := db.Delete(&MessageRecord{}, ids).Error; err != nil {
		return fmt.Errorf("mem: delete compacted: %w", err)
	}

	// Update session summary
	return db.Model(&SessionRecord{}).Where("id = ?", m.sessionID).
		Updates(map[string]interface{}{
			"summary":        summary,
			"summary_tokens": countTokens(summary),
			"updated_at":     time.Now(),
		}).Error
}

// dropOldestPair removes the oldest user+assistant pair (no-summarizer fallback).
func (m *PersistentMemory) dropOldestPair(db *gorm.DB) error {
	var oldest []MessageRecord
	if err := db.Where("session_id = ?", m.sessionID).Order("id asc").Limit(2).Find(&oldest).Error; err != nil {
		return err
	}
	if len(oldest) < 2 {
		return nil
	}
	return db.Delete(&MessageRecord{}, []uint{oldest[0].ID, oldest[1].ID}).Error
}

// countTokens estimates the token count of text using byte length / 3.
// Reasonable approximation for mixed Chinese/English (CJK chars: 3 UTF-8 bytes ≈ 1-2 tokens).
func countTokens(text string) int {
	n := len(text) / 3
	if n < 1 && len(text) > 0 {
		return 1
	}
	return n
}
