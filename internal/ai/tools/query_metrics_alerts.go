package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// PrometheusAlert 告警信息结构
type PrometheusAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	State       string            `json:"state"`
	ActiveAt    string            `json:"activeAt"`
	Value       string            `json:"value"`
}

// PrometheusAlertsResult 告警查询结果
type PrometheusAlertsResult struct {
	Status string `json:"status"`
	Data   struct {
		Alerts []PrometheusAlert `json:"alerts"`
	} `json:"data"`
	Error     string `json:"error,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
}

// SimplifiedAlert 简化的告警信息
type SimplifiedAlert struct {
	AlertName   string `json:"alert_name" jsonschema:"description=告警名称，从 Prometheus 告警的 labels.alertname 字段提取"`
	Description string `json:"description" jsonschema:"description=告警描述信息，从 Prometheus 告警的 annotations.description 字段提取"`
	State       string `json:"state" jsonschema:"description=告警状态，通常为 'firing'（触发中）或 'pending'（待触发）"`
	ActiveAt    string `json:"active_at" jsonschema:"description=告警激活时间，RFC3339 格式的时间戳，例如 '2025-10-29T08:48:42.496134755Z'"`
	Duration    string `json:"duration" jsonschema:"description=告警持续时间，从激活时间到当前时间的时长，格式如 '2h30m15s'、'30m15s' 或 '15s'"`
}

// PrometheusAlertsOutput 告警查询输出
type PrometheusAlertsOutput struct {
	Success bool              `json:"success" jsonschema:"description=查询是否成功"`
	Alerts  []SimplifiedAlert `json:"alerts,omitempty" jsonschema:"description=活动告警列表，每个告警包含名称、描述、状态、激活时间和持续时间。相同 alertname 的告警只保留第一个"`
	Message string            `json:"message,omitempty" jsonschema:"description=操作结果的状态消息"`
	Error   string            `json:"error,omitempty" jsonschema:"description=如果查询失败，包含错误信息"`
}

// queryPrometheusAlerts 查询Prometheus告警
func queryPrometheusAlerts() (PrometheusAlertsResult, error) {
	baseURL := "http://127.0.0.1:9090"
	apiURL := fmt.Sprintf("%s/api/v1/alerts", baseURL)

	log.Printf("Querying Prometheus alerts: %s", apiURL)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	var result PrometheusAlertsResult

	resp, err := client.Get(apiURL)
	if err != nil {
		return result, fmt.Errorf("failed to query Prometheus alerts: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, fmt.Errorf("failed to read response: %v", err)
	}

	if err = json.Unmarshal(body, &result); err != nil {
		return result, fmt.Errorf("failed to parse response: %v", err)
	}

	return result, nil
}

// calculateDuration 计算从 activeAt 到现在的持续时间
func calculateDuration(activeAtStr string) string {
	activeAt, err := time.Parse(time.RFC3339Nano, activeAtStr)
	if err != nil {
		return "unknown"
	}

	duration := time.Since(activeAt)

	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	} else {
		return fmt.Sprintf("%ds", seconds)
	}
}

// NewSecurityAlertsQueryTool 创建安全告警查询工具（IDS/IPS/WAF/防火墙告警）
func NewSecurityAlertsQueryTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_security_alerts",
		"查询当前活跃的安全告警，包括 IDS/IPS 入侵检测告警、WAF Web攻击告警、防火墙策略违规告警等。返回告警名称、威胁描述、状态、激活时间和持续时长。用于安全事件响应的第一步：了解当前安全态势。",
		func(ctx context.Context, input *struct{}, opts ...tool.Option) (output string, err error) {
			log.Printf("Querying Prometheus active alerts")

			result, err := queryPrometheusAlerts()
			if err != nil {
				alertsOut := PrometheusAlertsOutput{
					Success: false,
					Error:   err.Error(),
					Message: "Failed to query Prometheus alerts",
				}
				jsonBytes, _ := json.MarshalIndent(alertsOut, "", "  ")
				return string(jsonBytes), err
			}

			seenAlertNames := make(map[string]bool)
			simplifiedAlerts := make([]SimplifiedAlert, 0)
			for _, alert := range result.Data.Alerts {
				alertName := alert.Labels["alertname"]
				if seenAlertNames[alertName] {
					continue
				}
				seenAlertNames[alertName] = true
				simplifiedAlerts = append(simplifiedAlerts, SimplifiedAlert{
					AlertName:   alertName,
					Description: alert.Annotations["description"],
					State:       alert.State,
					ActiveAt:    alert.ActiveAt,
					Duration:    calculateDuration(alert.ActiveAt),
				})
			}

			alertsOut := PrometheusAlertsOutput{
				Success: true,
				Alerts:  simplifiedAlerts,
				Message: fmt.Sprintf("Successfully retrieved %d active alerts", len(simplifiedAlerts)),
			}

			jsonBytes, err := json.MarshalIndent(alertsOut, "", "  ")
			if err != nil {
				return "", fmt.Errorf("failed to marshal alerts result: %w", err)
			}

			log.Printf("Prometheus alerts query completed: %d alerts found", len(simplifiedAlerts))
			return string(jsonBytes), nil
		})
	if err != nil {
		log.Fatal(err)
	}
	return t
}
