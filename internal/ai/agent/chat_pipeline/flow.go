package chat_pipeline

import (
	"SecOpsAgent/internal/ai/tools"
	"context"

	"github.com/cloudwego/eino/components/tool"
)

func buildTools(_ context.Context) ([]tool.BaseTool, error) {
	mcpTool, err := tools.GetLogMcpTool()
	if err != nil {
		return nil, err
	}
	toolList := mcpTool
	toolList = append(toolList, tools.NewSecurityAlertsQueryTool())
	toolList = append(toolList, tools.NewQuerySecurityPlaybookTool())
	toolList = append(toolList, tools.NewQuerySecurityEventsTool())
	toolList = append(toolList, tools.NewQueryFirewallLogsTool())
	toolList = append(toolList, tools.NewGetCurrentTimeTool())
	return toolList, nil
}
