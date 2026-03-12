package mem

import (
	"fmt"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	dbOnce   sync.Once
	globalDB *gorm.DB
	dbErr    error
)

// SessionRecord holds per-user session metadata including the running compact summary.
type SessionRecord struct {
	ID            string    `gorm:"primaryKey"`
	Summary       string    `gorm:"type:text;not null;default:''"`
	SummaryTokens int       `gorm:"not null;default:0"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// MessageRecord stores a single user or assistant message for a session.
type MessageRecord struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	SessionID string    `gorm:"index;not null"`
	Role      string    `gorm:"not null"`
	Content   string    `gorm:"type:text;not null"`
	Tokens    int       `gorm:"not null;default:0"`
	CreatedAt time.Time `gorm:"not null"`
}

// Close 关闭底层 SQLite 连接，用于优雅关停时释放资源
func Close() {
	if globalDB != nil {
		sqlDB, err := globalDB.DB()
		if err == nil {
			sqlDB.Close()
		}
	}
}

func getDB() (*gorm.DB, error) {
	dbOnce.Do(func() {
		db, err := gorm.Open(sqlite.Open("agent_memory.db"), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			dbErr = fmt.Errorf("mem: open sqlite: %w", err)
			return
		}
		if err := db.AutoMigrate(&SessionRecord{}, &MessageRecord{}); err != nil {
			dbErr = fmt.Errorf("mem: migrate: %w", err)
			return
		}
		globalDB = db
	})
	return globalDB, dbErr
}
