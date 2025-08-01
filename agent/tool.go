package agent

import (
	"context"
	"log"
	"os/exec"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
)

type params struct {
	Command string `json:"command" description:"the command need to run"`
}

type result struct {
	Output  string `json:"output" description:"command output"`
	Success bool   `json:"success" description:"command execution success or not"`
}

func NewToolsNode(ctx context.Context) *compose.ToolsNode {
	tools := getTools()

	tn, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{Tools: tools})
	if err != nil {
		log.Fatal(err)
	}
	return tn
}

func executeCommand(ctx context.Context, params *params) (*result, error) {
	cmd := exec.Command("sh", "-c", params.Command)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return &result{
		Output:  string(output),
		Success: true,
	}, nil
}

func getTools() []tool.BaseTool {
	executeCommandTool, err := utils.InferTool(
		"execute_command",
		"input a command, run this command",
		executeCommand)
	if err != nil {
		log.Fatal(err)
	}

	return []tool.BaseTool{
		executeCommandTool,
	}
}
