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

// FirewallLog 防火墙日志模型（对应天融信 TopPolicy 防火墙策略日志）
type FirewallLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	SourceIP  string    `gorm:"index;size:45" json:"source_ip"`
	DestIP    string    `gorm:"index;size:45" json:"dest_ip"`
	DestPort  int       `gorm:"index" json:"dest_port"`
	Protocol  string    `gorm:"size:16" json:"protocol"`          // TCP, UDP, ICMP
	Action    string    `gorm:"index;size:16" json:"action"`      // allow, deny, drop
	RuleID    string    `gorm:"size:64" json:"rule_id"`           // 防火墙策略规则ID
	BytesSent int64     `json:"bytes_sent"`
	BytesRecv int64     `json:"bytes_recv"`
	Reason    string    `gorm:"size:256" json:"reason"`           // 命中原因
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// QueryFirewallLogsInput 防火墙日志查询输入
type QueryFirewallLogsInput struct {
	SourceIP  string `json:"source_ip,omitempty" jsonschema:"description=按源IP地址过滤防火墙日志"`
	DestIP    string `json:"dest_ip,omitempty" jsonschema:"description=按目标IP地址过滤防火墙日志"`
	Action    string `json:"action,omitempty" jsonschema:"description=按防火墙动作过滤,可选值: allow(放行), deny(拒绝), drop(丢弃)"`
	TimeRange string `json:"time_range,omitempty" jsonschema:"description=时间范围,可选值: 1h(最近1小时), 6h(最近6小时), 24h(最近24小时), 7d(最近7天),默认24h"`
	Limit     int    `json:"limit,omitempty" jsonschema:"description=返回结果数量上限,默认50"`
}

var firewallLogDB *gorm.DB

func getFirewallLogDB() (*gorm.DB, error) {
	if firewallLogDB != nil {
		return firewallLogDB, nil
	}
	db, err := gorm.Open(sqlite.Open("firewall_logs.db"), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to open firewall logs database: %w", err)
	}
	if err := db.AutoMigrate(&FirewallLog{}); err != nil {
		return nil, fmt.Errorf("failed to migrate firewall logs table: %w", err)
	}
	firewallLogDB = db
	return db, nil
}

// parseTimeRange 将时间范围字符串解析为 time.Duration
func parseTimeRange(tr string) time.Duration {
	switch tr {
	case "1h":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	default:
		return 24 * time.Hour // 默认24小时
	}
}

// NewQueryFirewallLogsTool 创建防火墙日志查询工具
func NewQueryFirewallLogsTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_firewall_logs",
		"查询天融信防火墙（TopPolicy）的流量日志，支持按源IP、目标IP、防火墙动作（allow/deny/drop）和时间范围过滤。返回匹配的防火墙会话日志，包含协议、端口、流量大小、命中规则等信息。用于安全事件溯源和攻击流量分析。",
		func(ctx context.Context, input *QueryFirewallLogsInput, opts ...tool.Option) (string, error) {
			db, err := getFirewallLogDB()
			if err != nil {
				return "", err
			}

			query := db.WithContext(ctx).Model(&FirewallLog{})

			// 时间范围过滤
			duration := parseTimeRange(input.TimeRange)
			since := time.Now().Add(-duration)
			query = query.Where("created_at >= ?", since)

			// 参数化条件构建
			if input.SourceIP != "" {
				query = query.Where("source_ip = ?", input.SourceIP)
			}
			if input.DestIP != "" {
				query = query.Where("dest_ip = ?", input.DestIP)
			}
			if input.Action != "" {
				query = query.Where("action = ?", input.Action)
			}

			limit := input.Limit
			if limit <= 0 || limit > 200 {
				limit = 50
			}

			var logs []FirewallLog
			if err := query.Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
				return "", fmt.Errorf("failed to query firewall logs: %w", err)
			}

			// 统计摘要
			var denyCount, dropCount, allowCount int
			for _, l := range logs {
				switch l.Action {
				case "deny":
					denyCount++
				case "drop":
					dropCount++
				case "allow":
					allowCount++
				}
			}

			result, err := json.MarshalIndent(map[string]interface{}{
				"success":    true,
				"count":      len(logs),
				"time_range": input.TimeRange,
				"summary": map[string]int{
					"allow": allowCount,
					"deny":  denyCount,
					"drop":  dropCount,
				},
				"logs": logs,
			}, "", "  ")
			if err != nil {
				return "", fmt.Errorf("failed to marshal results: %w", err)
			}

			log.Printf("Firewall logs query completed: %d logs found (allow=%d, deny=%d, drop=%d)",
				len(logs), allowCount, denyCount, dropCount)
			return string(result), nil
		})
	if err != nil {
		log.Fatal(err)
	}
	return t
}
