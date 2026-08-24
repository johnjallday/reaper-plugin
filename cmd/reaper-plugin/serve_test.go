package main

import (
	"context"
	"os/exec"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPrivateServiceConformsToMCPStdioAndDeclaredHealthOperation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", ".", "serve") // #nosec G204 -- fixed test command/arguments
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "reaper-plugin-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "service.version",
		Arguments: map[string]any{
			"protocol_version": 1,
			"operation_id":     "service.version",
			"context":          map[string]any{"workspace_id": "fixture"},
			"input":            map[string]any{},
		},
	})
	if err != nil || result.IsError {
		t.Fatalf("service.version = %+v, %v", result, err)
	}
	content, ok := result.StructuredContent.(map[string]any)
	if !ok || content["name"] != "reaper-plugin" || content["protocol_version"] != float64(1) || content["healthy"] != true {
		t.Fatalf("structured health = %#v", result.StructuredContent)
	}
}
