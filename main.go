package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Fl0rencess720/MiniCli/agent"
)

func main() {
	ctx := context.Background()
	tpl := agent.NewChatTemplate(ctx)
	cm := agent.NewChatModel(ctx)
	store := agent.NewCheckPointStore(ctx)
	tools := agent.NewToolsNode(ctx)
	agent, err := agent.NewCliAgent(ctx, tpl, cm, tools, store)
	if err != nil {
		log.Fatal(err)
	}

	var input string
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print(">>> ")
		if scanner.Scan() {
			input = scanner.Text()
		}
		err = agent.Run(ctx, input)
		if err != nil {
			log.Fatal(err)
		}
	}

}
