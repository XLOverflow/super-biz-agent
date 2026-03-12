package chat

import (
	"SecOpsAgent/api/chat/v1"
	"SecOpsAgent/internal/ai/agent/plan_execute_replan"
	"context"
	"errors"
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
	return res, nil

}
