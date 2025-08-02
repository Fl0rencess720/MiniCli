package agent

import (
	"context"

	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
)

func NewChatTemplate(_ context.Context) prompt.ChatTemplate {
	return prompt.FromMessages(schema.FString,
		schema.SystemMessage("你是一个命令行助手，当用户向你提出相关要求时，调用相关工具完成此任务"),
		schema.MessagesPlaceholder("chat_history", true),
		schema.UserMessage("{user_input}"),
	)
}
