package models

import (
	"context"
	"os"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/gogf/gf/v2/frame/g"
)

func cfgStr(ctx context.Context, key string) (string, error) {
	v, err := g.Cfg().Get(ctx, key)
	if err != nil {
		return "", err
	}
	return os.ExpandEnv(v.String()), nil
}

func OpenAIForDeepSeekV31Think(ctx context.Context) (cm model.ToolCallingChatModel, err error) {
	modelName, err := cfgStr(ctx, "ds_think_chat_model.model")
	if err != nil {
		return nil, err
	}
	apiKey, err := cfgStr(ctx, "ds_think_chat_model.api_key")
	if err != nil {
		return nil, err
	}
	baseURL, err := cfgStr(ctx, "ds_think_chat_model.base_url")
	if err != nil {
		return nil, err
	}
	cm, err = openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:   modelName,
		APIKey:  apiKey,
		BaseURL: baseURL,
		ExtraFields: map[string]any{
			"thinking": map[string]any{"type": "enabled"},
		},
	})
	return
}

func OpenAIForDeepSeekV3Quick(ctx context.Context) (cm model.ToolCallingChatModel, err error) {
	modelName, err := cfgStr(ctx, "ds_quick_chat_model.model")
	if err != nil {
		return nil, err
	}
	apiKey, err := cfgStr(ctx, "ds_quick_chat_model.api_key")
	if err != nil {
		return nil, err
	}
	baseURL, err := cfgStr(ctx, "ds_quick_chat_model.base_url")
	if err != nil {
		return nil, err
	}
	cm, err = openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:   modelName,
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
	return
}
