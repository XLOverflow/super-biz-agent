package chat_pipeline

import (
	"SuperBizAgent/internal/ai/tools"
	"context"

	"github.com/cloudwego/eino/components/tool"
)

func buildTools(_ context.Context) ([]tool.BaseTool, error) {
	mcpTool, err := tools.GetLogMcpTool()
	if err != nil {
		return nil, err
	}
	toolList := mcpTool
	toolList = append(toolList, tools.NewPrometheusAlertsQueryTool())
	toolList = append(toolList, tools.NewMysqlCrudTool())
	toolList = append(toolList, tools.NewGetCurrentTimeTool())
	toolList = append(toolList, tools.NewQueryInternalDocsTool())
	return toolList, nil
}
