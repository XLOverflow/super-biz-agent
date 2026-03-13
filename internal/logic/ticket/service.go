package ticket

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var (
	ticketDB   *gorm.DB
	ticketOnce sync.Once
	ticketErr  error
)

func getDB() (*gorm.DB, error) {
	ticketOnce.Do(func() {
		db, err := gorm.Open(sqlite.Open("tickets.db"), &gorm.Config{})
		if err != nil {
			ticketErr = fmt.Errorf("ticket: open db: %w", err)
			return
		}
		if err := db.AutoMigrate(&Ticket{}, &OnCallEntry{}); err != nil {
			ticketErr = fmt.Errorf("ticket: migrate: %w", err)
			return
		}
		ticketDB = db
	})
	return ticketDB, ticketErr
}

// Close 关闭工单数据库连接
func Close() {
	if ticketDB != nil {
		sqlDB, err := ticketDB.DB()
		if err == nil {
			sqlDB.Close()
		}
	}
}

// CreateRequest 创建工单的请求参数
type CreateRequest struct {
	EventType       string   // 事件类型
	Severity        Severity // 严重等级
	Title           string   // 工单标题
	AnalysisSummary string   // Agent 分析摘要
	SuggestedAction string   // 建议处置动作
	AlertSource     string   // 告警来源
	SourceIP        string   // 攻击源 IP
	CreatedBy       string   // 创建者
}

// CreateAndDispatch 创建工单并自动派发。
// 1. 生成工单编号
// 2. 根据严重等级查值班表确定派发对象
// 3. 写入数据库
// 返回创建的工单。
func CreateAndDispatch(req *CreateRequest) (*Ticket, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	// 生成工单编号: SEC-{年份}-{月日}-{序号}
	ticketNo := generateTicketNo(db)

	// 根据严重等级确定派发角色和值班人员
	role, assignee := dispatch(db, req.Severity)

	ticket := &Ticket{
		TicketNo:        ticketNo,
		EventType:       req.EventType,
		Severity:        req.Severity,
		Status:          StatusAssigned,
		Title:           req.Title,
		AnalysisSummary: req.AnalysisSummary,
		SuggestedAction: req.SuggestedAction,
		AlertSource:     req.AlertSource,
		SourceIP:        req.SourceIP,
		AssignedTo:      assignee,
		AssignedRole:    role,
		CreatedBy:       req.CreatedBy,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := db.Create(ticket).Error; err != nil {
		return nil, fmt.Errorf("ticket: create: %w", err)
	}

	log.Printf("ticket: created %s [%s] assigned to %s(%s) — %s",
		ticketNo, req.Severity, assignee, role, req.Title)
	return ticket, nil
}

// UpdateStatus 更新工单状态
func UpdateStatus(ticketNo string, status Status) error {
	db, err := getDB()
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}
	if status == StatusResolved {
		now := time.Now()
		updates["resolved_at"] = &now
	}
	return db.Model(&Ticket{}).Where("ticket_no = ?", ticketNo).Updates(updates).Error
}

// GetByTicketNo 根据工单编号查询
func GetByTicketNo(ticketNo string) (*Ticket, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}
	var t Ticket
	if err := db.Where("ticket_no = ?", ticketNo).First(&t).Error; err != nil {
		return nil, fmt.Errorf("ticket: not found: %w", err)
	}
	return &t, nil
}

// ListPending 列出所有待处理的工单，按严重等级排序
func ListPending() ([]Ticket, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}
	var tickets []Ticket
	err = db.Where("status IN ?", []Status{StatusPending, StatusAssigned}).
		Order("CASE severity WHEN 'P0' THEN 0 WHEN 'P1' THEN 1 WHEN 'P2' THEN 2 WHEN 'P3' THEN 3 END").
		Find(&tickets).Error
	return tickets, err
}

// IsDuplicateAlert 告警去重：检查同一源 IP + 同一事件类型在指定时间窗口内是否已有工单。
// 避免同一告警短时间内生成大量重复工单。
func IsDuplicateAlert(sourceIP, eventType string, windowMinutes int) bool {
	db, err := getDB()
	if err != nil {
		return false
	}
	since := time.Now().Add(-time.Duration(windowMinutes) * time.Minute)
	var count int64
	db.Model(&Ticket{}).
		Where("source_ip = ? AND event_type = ? AND created_at >= ?", sourceIP, eventType, since).
		Count(&count)
	return count > 0
}

// generateTicketNo 生成工单编号 SEC-2024-0312-001
func generateTicketNo(db *gorm.DB) string {
	now := time.Now()
	dateStr := now.Format("0102") // MMDD
	yearStr := now.Format("2006")

	// 查询今天已有的工单数量作为序号
	var count int64
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	db.Model(&Ticket{}).Where("created_at >= ?", todayStart).Count(&count)

	return fmt.Sprintf("SEC-%s-%s-%03d", yearStr, dateStr, count+1)
}

// dispatch 根据严重等级确定派发角色和值班人员
func dispatch(db *gorm.DB, severity Severity) (role string, assignee string) {
	// 派发规则：
	// P0(紧急) → L3 安全主管 + 通知 L2 值班
	// P1(高危) → L2 当前值班工程师
	// P2(中危) → L2 工单队列
	// P3(低危) → L1 工单队列
	switch severity {
	case SeverityP0:
		role = "L3"
	case SeverityP1, SeverityP2:
		role = "L2"
	default:
		role = "L1"
	}

	// 查值班表获取当天该角色的值班人员
	today := time.Now().Format("2006-01-02")
	var entry OnCallEntry
	if err := db.Where("date = ? AND role = ?", today, role).First(&entry).Error; err == nil {
		assignee = entry.UserName
	} else {
		// 值班表无数据时使用默认值（生产环境应有兜底机制）
		assignee = fmt.Sprintf("%s-oncall", role)
	}

	return role, assignee
}
