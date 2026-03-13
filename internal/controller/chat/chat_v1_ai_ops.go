package chat

import (
	v1 "SecOpsAgent/api/chat/v1"
	"SecOpsAgent/internal/ai/agent/plan_execute_replan"
	"SecOpsAgent/internal/logic/flywheel"
	"SecOpsAgent/internal/logic/ticket"
	"context"
	"errors"
	"log"
	"strings"
)

func (c *ControllerV1) AIOps(ctx context.Context, req *v1.AIOpsReq) (res *v1.AIOpsRes, err error) {
	query := `
"1. 你是天融信SOC的安全事件响应助手。首先调用工具 query_security_alerts 获取所有活跃的安全告警。"
"2. 针对每个告警，调用工具 query_security_playbook 检索对应的安全处置 Playbook。"
"3. 严格遵循内部安全 Playbook 的处置流程，不使用文档外的任何处置建议。"
"4. 涉及时间参数时，先通过 get_current_time 获取当前时间，再进行时间范围计算。"
"5. 如需查询防火墙日志，调用 query_firewall_logs 获取相关流量日志，分析攻击源IP、目标端口和动作。"
"6. 综合告警信息、Playbook、日志分析结果，生成安全事件响应报告，格式如下：
安全事件响应报告
---
# 事件概览
## 活跃安全告警清单（含威胁等级P0-P3）
## 威胁分析N（第N个告警）
  - 攻击向量与MITRE ATT&CK映射
  - 影响范围评估
  - 关联IOC指标（IP/域名/哈希）
## 处置执行N（第N个告警）
  - 已执行的自动化处置步骤
  - 需人工介入的处置项
## 总结与建议
  - 整体安全态势评估
  - 后续加固建议
`

	resp, detail, err := plan_execute_replan.BuildPlanAgent(ctx, query)
	if err != nil {
		return nil, err
	}
	if resp == "" {
		return nil, errors.New("内部错误")
	}

	res = &v1.AIOpsRes{
		Result: resp,
		Detail: detail,
	}

	// ---- 后处理：工单自动生成 + 知识飞轮（异步，不阻塞响应） ----
	go postProcess(context.Background(), resp, detail, res)

	return res, nil
}

// postProcess AIOps 后处理：自动生成工单 + 将处置记录索引回知识库。
// 在独立 goroutine 中执行，不阻塞 HTTP 响应返回。
func postProcess(ctx context.Context, result string, detail []string, res *v1.AIOpsRes) {
	// === 1. 自动生成工单 ===
	// 从 Agent 分析结果中提取关键信息创建工单。
	// 生产环境会用 NLP/正则从 result 中提取事件类型、源IP等结构化字段，
	// 这里简化为以整个分析报告作为摘要创建汇总工单。
	severity := extractSeverity(result)
	ticketReq := &ticket.CreateRequest{
		EventType:       "security_incident",
		Severity:        severity,
		Title:           "安全事件自动响应报告",
		AnalysisSummary: truncate(result, 2048),
		SuggestedAction: "请查看分析报告中的处置建议并确认执行",
		AlertSource:     "AIOps-Agent",
		CreatedBy:       "system",
	}

	// 告警去重：同一类型5分钟内不重复生成工单
	if !ticket.IsDuplicateAlert(ticketReq.SourceIP, ticketReq.EventType, 5) {
		t, err := ticket.CreateAndDispatch(ticketReq)
		if err != nil {
			log.Printf("postprocess: create ticket failed: %v", err)
		} else {
			log.Printf("postprocess: ticket %s created, assigned to %s(%s)",
				t.TicketNo, t.AssignedTo, t.AssignedRole)
			// 将工单编号回写到响应（goroutine 内写入，取决于响应是否已发送）
			res.TicketNo = t.TicketNo
		}
	} else {
		log.Printf("postprocess: duplicate alert suppressed (event_type=%s)", ticketReq.EventType)
	}

	// === 2. 知识飞轮：将处置记录索引回 RAG 知识库 ===
	fwService, err := flywheel.NewService(ctx)
	if err != nil {
		log.Printf("postprocess: init flywheel failed: %v", err)
		return
	}
	record := &flywheel.Record{
		EventType: "security_incident",
		Severity:  string(severity),
		Summary:   truncate(result, 4096),
		Actions:   joinDetail(detail),
		Result:    "AIOps 自动分析完成",
	}
	docID, isDup, err := fwService.Ingest(ctx, record)
	if err != nil {
		log.Printf("postprocess: flywheel ingest failed: %v", err)
	} else if isDup {
		log.Printf("postprocess: flywheel skipped duplicate record")
	} else {
		log.Printf("postprocess: flywheel indexed as %s", docID)
	}
}

// extractSeverity 从分析报告中提取最高严重等级
func extractSeverity(report string) ticket.Severity {
	if strings.Contains(report, "P0") {
		return ticket.SeverityP0
	}
	if strings.Contains(report, "P1") {
		return ticket.SeverityP1
	}
	if strings.Contains(report, "P2") {
		return ticket.SeverityP2
	}
	if strings.Contains(report, "P3") {
		return ticket.SeverityP3
	}
	return ticket.SeverityP2 // 默认中危
}

// truncate 截断字符串到指定 rune 长度
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// joinDetail 将执行细节拼接为单个字符串
func joinDetail(detail []string) string {
	return truncate(strings.Join(detail, "\n---\n"), 4096)
}
