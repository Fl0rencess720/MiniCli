package agent

import (
	"context"

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

func NewCheckPointStore(ctx context.Context) compose.CheckPointStore {
	return &cliStore{buf: make(map[string][]byte)}
}

func buildCliGraph(tpl prompt.ChatTemplate, cm model.ToolCallingChatModel, tn *compose.ToolsNode, store compose.CheckPointStore) (*compose.Graph[map[string]any, *schema.Message], error) {
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
	err = g.AddPassthroughNode("UserDecision")
	if err != nil {
		return nil, err
	}
	err = g.AddLambdaNode("ToolsCancelLambda", compose.InvokableLambda(func(ctx context.Context, input *schema.Message) ([]*schema.Message, error) {

		return []*schema.Message{
			schema.UserMessage("用户已取消本次工具调用，请你正常与用户继续交流以获取用户的后续需求"),
		}, nil
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
	err = g.AddBranch("ChatModel", compose.NewGraphBranch(func(ctx context.Context, in *schema.Message) (endNode string, err error) {
		if len(in.ToolCalls) > 0 {
			return "UserDecision", nil
		}
		return compose.END, nil
	}, map[string]bool{"UserDecision": true, compose.END: true}))
	if err != nil {
		return nil, err
	}
	err = g.AddBranch("UserDecision", compose.NewGraphBranch(func(ctx context.Context, in *schema.Message) (endNode string, err error) {
		history := []*schema.Message{}
		if err := compose.ProcessState(ctx, func(ctx context.Context, state *cliState) error {
			history = state.history
			return nil
		}); err != nil {
			return "", err
		}
		if len(history[len(history)-1].ToolCalls) > 0 {
			return "ToolsNode", nil
		}
		return "ToolsCancelLambda", nil
	}, map[string]bool{"ToolsCancelLambda": true, "ToolsNode": true}))
	if err != nil {
		return nil, err
	}
	err = g.AddEdge("ToolsCancelLambda", "ChatModel")
	if err != nil {
		return nil, err
	}
	err = g.AddEdge("ToolsNode", "ChatModel")
	if err != nil {
		return nil, err
	}
	return g, nil
}
