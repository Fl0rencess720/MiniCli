package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type cliAgent struct {
	runnable            compose.Runnable[map[string]any, *schema.Message]
	conversationHistory []*schema.Message
}

type cliState struct {
	history []*schema.Message
}

type cliStore struct {
	buf map[string][]byte
}

func (m *cliStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	data, ok := m.buf[checkPointID]
	return data, ok, nil
}

func (m *cliStore) Set(ctx context.Context, checkPointID string, checkPoint []byte) error {
	m.buf[checkPointID] = checkPoint
	return nil
}

func NewChatTemplate(_ context.Context) prompt.ChatTemplate {
	return prompt.FromMessages(schema.FString,
		schema.SystemMessage("你是一个命令行助手，当用户向你提出相关要求时，调用相关工具完成此任务"),
		schema.MessagesPlaceholder("chat_history", true),
		schema.UserMessage("{user_input}"),
	)
}

func NewChatModel(ctx context.Context) model.ToolCallingChatModel {
	cm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		Model:   os.Getenv("OPENAI_MODEL"),
		BaseURL: os.Getenv("OPENAI_BASE_URL"),
	})
	if err != nil {
		log.Fatal(err)
	}

	tools := getTools()
	var toolsInfo []*schema.ToolInfo
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			log.Fatal(err)
		}
		toolsInfo = append(toolsInfo, info)
	}

	err = cm.BindTools(toolsInfo)
	if err != nil {
		log.Fatal(err)
	}
	return cm
}
func NewCheckPointStore(ctx context.Context) compose.CheckPointStore {
	return &cliStore{buf: make(map[string][]byte)}
}

func NewCliAgent(ctx context.Context, tpl prompt.ChatTemplate, cm model.ToolCallingChatModel, tn *compose.ToolsNode, store compose.CheckPointStore) (*cliAgent, error) {
	compose.RegisterSerializableType[cliState]("state")
	g := compose.NewGraph[map[string]any, *schema.Message](
		compose.WithGenLocalState(func(ctx context.Context) *cliState {
			return &cliState{}
		}))
	err := g.AddChatTemplateNode(
		"ChatTemplate",
		tpl,
	)
	if err != nil {
		return nil, err
	}
	err = g.AddChatModelNode(
		"ChatModel",
		cm,
		compose.WithStatePreHandler(func(ctx context.Context, in []*schema.Message, state *cliState) ([]*schema.Message, error) {
			state.history = append(state.history, in...)
			return state.history, nil
		}),
		compose.WithStatePostHandler(func(ctx context.Context, out *schema.Message, state *cliState) (*schema.Message, error) {
			state.history = append(state.history, out)
			return out, nil
		}),
	)
	if err != nil {
		return nil, err
	}
	err = g.AddToolsNode("ToolsNode", tn, compose.WithStatePreHandler(func(ctx context.Context, in *schema.Message, state *cliState) (*schema.Message, error) {
		if len(state.history) == 0 {
			return in, nil
		}
		return state.history[len(state.history)-1], nil
	}))
	if err != nil {
		return nil, err
	}

	err = g.AddEdge(compose.START, "ChatTemplate")
	if err != nil {
		return nil, err
	}
	err = g.AddEdge("ChatTemplate", "ChatModel")
	if err != nil {
		return nil, err
	}
	err = g.AddEdge("ToolsNode", "ChatModel")
	if err != nil {
		return nil, err
	}
	err = g.AddBranch("ChatModel", compose.NewGraphBranch(func(ctx context.Context, in *schema.Message) (endNode string, err error) {
		if len(in.ToolCalls) > 0 {
			return "ToolsNode", nil
		}
		return compose.END, nil
	}, map[string]bool{"ToolsNode": true, compose.END: true}))
	if err != nil {
		return nil, err
	}
	runnable, err := g.Compile(
		ctx,
		compose.WithCheckPointStore(store),
		compose.WithInterruptBeforeNodes([]string{"ToolsNode"}),
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
			fmt.Print("你是否要执行以下命令? (y/n): ")
			var response string
			fmt.Scanln(&response)

			if strings.ToLower(response) == "n" {
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
			}
		}
	}
	return nil
}
