package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// SecurityEvent 安全事件表模型
type SecurityEvent struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	EventType   string    `gorm:"index;size:64" json:"event_type"`   // brute_force, port_scan, malware, unauthorized_access, ddos
	Severity    string    `gorm:"size:16" json:"severity"`           // P0, P1, P2, P3
	SourceIP    string    `gorm:"index;size:45" json:"source_ip"`
	DestIP      string    `gorm:"index;size:45" json:"dest_ip"`
	DestPort    int       `json:"dest_port"`
	Description string    `gorm:"size:1024" json:"description"`
	Status      string    `gorm:"size:32" json:"status"`             // open, investigating, resolved
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

// QuerySecurityEventsInput 安全事件查询输入
type QuerySecurityEventsInput struct {
	EventType string `json:"event_type,omitempty" jsonschema:"description=安全事件类型过滤,可选值: brute_force(暴力破解), port_scan(端口扫描), malware(恶意软件), unauthorized_access(未授权访问), ddos(拒绝服务)"`
	Severity  string `json:"severity,omitempty" jsonschema:"description=威胁等级过滤,可选值: P0(紧急), P1(高危), P2(中危), P3(低危)"`
	SourceIP  string `json:"source_ip,omitempty" jsonschema:"description=按攻击源IP地址过滤"`
	Status    string `json:"status,omitempty" jsonschema:"description=事件状态过滤,可选值: open(待处理), investigating(调查中), resolved(已解决)"`
	Limit     int    `json:"limit,omitempty" jsonschema:"description=返回结果数量上限,默认20"`
}

var securityEventsDB *gorm.DB

func getSecurityEventsDB() (*gorm.DB, error) {
	if securityEventsDB != nil {
		return securityEventsDB, nil
	}
	db, err := gorm.Open(sqlite.Open("security_events.db"), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to open security events database: %w", err)
	}
	if err := db.AutoMigrate(&SecurityEvent{}); err != nil {
		return nil, fmt.Errorf("failed to migrate security events table: %w", err)
	}
	securityEventsDB = db
	return db, nil
}

// NewQuerySecurityEventsTool 创建安全事件查询工具（参数化查询，防止SQL注入）
func NewQuerySecurityEventsTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_security_events",
		"查询安全事件数据库，支持按事件类型、威胁等级、攻击源IP、事件状态进行参数化过滤。返回匹配的安全事件列表，包含事件详情和处理状态。所有查询均使用参数化方式执行，防止SQL注入。",
		func(ctx context.Context, input *QuerySecurityEventsInput, opts ...tool.Option) (string, error) {
			db, err := getSecurityEventsDB()
			if err != nil {
				return "", err
			}

			query := db.WithContext(ctx).Model(&SecurityEvent{})

			// 参数化条件构建，避免SQL注入
			if input.EventType != "" {
				query = query.Where("event_type = ?", input.EventType)
			}
			if input.Severity != "" {
				query = query.Where("severity = ?", input.Severity)
			}
			if input.SourceIP != "" {
				query = query.Where("source_ip = ?", input.SourceIP)
			}
			if input.Status != "" {
				query = query.Where("status = ?", input.Status)
			}

			limit := input.Limit
			if limit <= 0 || limit > 100 {
				limit = 20
			}

			var events []SecurityEvent
			if err := query.Order("created_at DESC").Limit(limit).Find(&events).Error; err != nil {
				return "", fmt.Errorf("failed to query security events: %w", err)
			}

			result, err := json.MarshalIndent(map[string]interface{}{
				"success": true,
				"count":   len(events),
				"events":  events,
			}, "", "  ")
			if err != nil {
				return "", fmt.Errorf("failed to marshal results: %w", err)
			}

			log.Printf("Security events query completed: %d events found", len(events))
			return string(result), nil
		})
	if err != nil {
		log.Fatal(err)
	}
	return t
}
