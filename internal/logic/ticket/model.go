package ticket

import (
	"time"
)

// Status 工单状态
type Status string

const (
	StatusPending      Status = "pending"       // 待处理
	StatusAssigned     Status = "assigned"       // 已派发
	StatusInProgress   Status = "in_progress"    // 处理中
	StatusResolved     Status = "resolved"       // 已处理
	StatusClosed       Status = "closed"         // 已关闭
)

// Severity 严重等级
type Severity string

const (
	SeverityP0 Severity = "P0" // 紧急：15分钟内响应
	SeverityP1 Severity = "P1" // 高危：30分钟内响应
	SeverityP2 Severity = "P2" // 中危：2小时内响应
	SeverityP3 Severity = "P3" // 低危：24小时内响应
)

// Ticket 安全事件工单模型
type Ticket struct {
	ID              uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	TicketNo        string    `gorm:"uniqueIndex;size:32;not null" json:"ticket_no"`          // 工单编号 SEC-2024-0312-001
	EventType       string    `gorm:"index;size:64;not null" json:"event_type"`               // 事件类型
	Severity        Severity  `gorm:"index;size:8;not null" json:"severity"`                  // 严重等级
	Status          Status    `gorm:"index;size:16;not null;default:'pending'" json:"status"`  // 工单状态
	Title           string    `gorm:"size:256;not null" json:"title"`                         // 工单标题
	AnalysisSummary string    `gorm:"type:text" json:"analysis_summary"`                      // Agent 分析摘要
	SuggestedAction string    `gorm:"type:text" json:"suggested_action"`                      // 建议处置动作
	AlertSource     string    `gorm:"size:128" json:"alert_source"`                           // 告警来源（IDS/WAF/FW）
	SourceIP        string    `gorm:"index;size:45" json:"source_ip"`                         // 攻击源 IP
	AssignedTo      string    `gorm:"index;size:64" json:"assigned_to"`                       // 派发给谁
	AssignedRole    string    `gorm:"size:16" json:"assigned_role"`                           // 派发角色（L1/L2/L3）
	CreatedBy       string    `gorm:"size:64" json:"created_by"`                              // 创建者（system/user_id）
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`                                 // 解决时间
	CreatedAt       time.Time `gorm:"index;not null" json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// OnCallEntry 值班表条目
type OnCallEntry struct {
	ID       uint     `gorm:"primaryKey;autoIncrement" json:"id"`
	Date     string   `gorm:"index;size:10;not null" json:"date"`      // 2024-03-12
	Role     string   `gorm:"size:16;not null" json:"role"`            // L1, L2, L3
	UserID   string   `gorm:"size:64;not null" json:"user_id"`        // 值班人员ID
	UserName string   `gorm:"size:64;not null" json:"user_name"`      // 值班人员姓名
}
