package plan_execute_replan

import (
	"SecOpsAgent/internal/ai/models"
	"SecOpsAgent/internal/ai/tools"
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/compose"
)

func NewExecutor(ctx context.Context) (adk.Agent, error) {
	// log (MCP)
	mcpTool, err := tools.GetLogMcpTool()
	if err != nil {
		return nil, err
	}
	toolList := mcpTool
	// security alerts
	toolList = append(toolList, tools.NewSecurityAlertsQueryTool())
	// security playbook
	toolList = append(toolList, tools.NewQuerySecurityPlaybookTool())
	// firewall logs
	toolList = append(toolList, tools.NewQueryFirewallLogsTool())
	// security events db
	toolList = append(toolList, tools.NewQuerySecurityEventsTool())
	// time
	toolList = append(toolList, tools.NewGetCurrentTimeTool())
	execModel, err := models.OpenAIForDeepSeekV3Quick(ctx)
	if err != nil {
		return nil, err
	}
	return planexecute.NewExecutor(ctx, &planexecute.ExecutorConfig{
		Model: execModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: toolList,
			},
		},
		MaxIterations: 999999,
	})
}
