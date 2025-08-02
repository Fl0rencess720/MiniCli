package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func NewCliAgent(ctx context.Context, tpl prompt.ChatTemplate, cm model.ToolCallingChatModel, tn *compose.ToolsNode, store compose.CheckPointStore) (*cliAgent, error) {
	g, err := buildCliGraph(tpl, cm, tn, store)
	if err != nil {
		return nil, err
	}
	runnable, err := g.Compile(
		ctx,
		compose.WithCheckPointStore(store),
		compose.WithInterruptBeforeNodes([]string{"UserDecision"}),
	)
	if err != nil {
		return nil, err
	}
	return &cliAgent{
		runnable:            runnable,
		conversationHistory: make([]*schema.Message, 0),
	}, nil
}

func (a *cliAgent) Run(ctx context.Context, message string) (err error) {
	var history []*schema.Message

	checkpointID := fmt.Sprintf("%d", time.Now().UnixNano())
	for {
		input := map[string]any{
			"user_input":   message,
			"chat_history": a.conversationHistory,
		}
		result, err := a.runnable.Invoke(ctx, input,
			compose.WithCheckPointID(checkpointID),
			compose.WithStateModifier(func(ctx context.Context, path compose.NodePath, state any) error {
				state.(*cliState).history = history
				return nil
			}))

		if err == nil {
			a.conversationHistory = append(a.conversationHistory,
				schema.UserMessage(message),
				result)
			fmt.Printf("%s\n", result.Content)
			break
		}

		info, ok := compose.ExtractInterruptInfo(err)
		if !ok {
			log.Fatal(err)
		}

		history = info.State.(*cliState).history

		for i, tc := range history[len(history)-1].ToolCalls {

			toolArgs := params{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &toolArgs); err != nil {
				log.Fatal(err)
			}
			fmt.Printf("模型将执行以下命令: %s\n", toolArgs.Command)
			fmt.Print("请输入响应的数字选择下一步：\n1、执行该命令\n2、修改该命令:\n3、拒绝执行\n ")
			var response string
			fmt.Scanln(&response)

			switch response {
			case "1":
			case "2":
				fmt.Print("请输入新的命令: ")
				scanner := bufio.NewScanner(os.Stdin)
				var newCommand string
				if scanner.Scan() {
					newCommand = scanner.Text()
				}
				newArgs := params{Command: newCommand}
				newArgsByte, err := json.Marshal(newArgs)
				if err != nil {
					log.Fatal(err)
				}
				history[len(history)-1].ToolCalls[i].Function.Arguments = string(newArgsByte)
				fmt.Printf("命令更新为: %s\n", newCommand)
			case "3":
				history[len(history)-1].ToolCalls = nil
			default:

			}
		}
	}
	return nil
}
