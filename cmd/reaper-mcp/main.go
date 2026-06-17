package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/johnjallday/reaper-mcp/internal/mcpio"
	"github.com/johnjallday/reaper-mcp/internal/reaper"
)

const (
	protocolVersion = "2024-11-05"
	serverName      = "reaper-mcp"
	serverVersion   = "0.1.0"
	toolName        = "ori-reaper"
)

func main() {
	manager := reaper.NewManagerFromEnv()
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)

	for {
		raw, framing, err := mcpio.ReadMessage(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			logErr(fmt.Errorf("read message: %w", err))
			continue
		}

		var req mcpio.Request
		if err := json.Unmarshal(raw, &req); err != nil {
			logErr(fmt.Errorf("parse request: %w", err))
			_ = writeRPCError(writer, framing, nil, mcpio.ParseError, "invalid JSON")
			continue
		}

		if strings.TrimSpace(req.Method) == "" {
			_ = writeRPCError(writer, framing, req.ID, mcpio.InvalidRequest, "missing method")
			continue
		}

		if req.ID == nil {
			// Notification path.
			continue
		}

		switch req.Method {
		case mcpio.MethodInitialize:
			result := mcpio.InitializeResult{
				ProtocolVersion: protocolVersion,
				Capabilities: mcpio.ServerCapabilities{
					Tools: &mcpio.ToolsCapability{ListChanged: false},
				},
				ServerInfo: mcpio.Implementation{Name: serverName, Version: serverVersion},
			}
			_ = writeRPCResult(writer, framing, req.ID, result)

		case mcpio.MethodToolsList:
			result := mcpio.ToolsListResult{Tools: []mcpio.Tool{reaperToolDefinition()}}
			_ = writeRPCResult(writer, framing, req.ID, result)

		case mcpio.MethodToolsCall:
			var params mcpio.ToolCallParams
			if len(req.Params) > 0 {
				if err := json.Unmarshal(req.Params, &params); err != nil {
					_ = writeRPCError(writer, framing, req.ID, mcpio.InvalidParams, "invalid tools/call params")
					continue
				}
			}

			if strings.TrimSpace(params.Name) != toolName {
				toolResult := mcpio.ToolCallResult{
					IsError: true,
					Content: []mcpio.ContentItem{{Type: "text", Text: fmt.Sprintf("unknown tool: %s", params.Name)}},
				}
				_ = writeRPCResult(writer, framing, req.ID, toolResult)
				continue
			}

			var opParams reaper.Params
			if len(params.Arguments) > 0 {
				payload, _ := json.Marshal(params.Arguments)
				if err := json.Unmarshal(payload, &opParams); err != nil {
					toolResult := mcpio.ToolCallResult{
						IsError: true,
						Content: []mcpio.ContentItem{{Type: "text", Text: fmt.Sprintf("invalid arguments: %v", err)}},
					}
					_ = writeRPCResult(writer, framing, req.ID, toolResult)
					continue
				}
			}

			text, err := manager.Execute(opParams)
			if err != nil {
				toolResult := mcpio.ToolCallResult{
					IsError: true,
					Content: []mcpio.ContentItem{{Type: "text", Text: err.Error()}},
				}
				_ = writeRPCResult(writer, framing, req.ID, toolResult)
				continue
			}

			toolResult := mcpio.ToolCallResult{
				Content: []mcpio.ContentItem{{Type: "text", Text: text}},
			}
			_ = writeRPCResult(writer, framing, req.ID, toolResult)

		case mcpio.MethodPing:
			_ = writeRPCResult(writer, framing, req.ID, map[string]any{})

		default:
			_ = writeRPCError(writer, framing, req.ID, mcpio.MethodNotFound, fmt.Sprintf("unknown method: %s", req.Method))
		}
	}
}

func reaperToolDefinition() mcpio.Tool {
	return mcpio.Tool{
		Name:        toolName,
		Description: "Manage REAPER ReaScripts, status, project context, and tracks using operation-based arguments.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{
					"type":        "string",
					"description": "Operation to perform",
					"enum": []string{
						"list",
						"run",
						"add",
						"delete",
						"list_available_scripts",
						"download_script",
						"register_script",
						"register_all_scripts",
						"clean_scripts",
						"get_context",
						"get_status",
						"get_web_remote_port",
						"get_tracks",
					},
				},
				"script": map[string]any{
					"type":        "string",
					"description": "Script name (with or without extension)",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Script source code for add operation",
				},
				"script_type": map[string]any{
					"type":        "string",
					"description": "Script type for add operation",
					"enum":        []string{"lua", "eel", "py"},
				},
				"filename": map[string]any{
					"type":        "string",
					"description": "Optional full filename",
				},
			},
			"required": []string{"operation"},
		},
	}
}

func writeRPCResult(writer *bufio.Writer, framing mcpio.FramingMode, id any, result any) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	resp := mcpio.Response{JSONRPC: mcpio.JSONRPCVersion, ID: id, Result: raw}
	if err := mcpio.WriteMessage(writer, resp, framing); err != nil {
		return err
	}
	return writer.Flush()
}

func writeRPCError(writer *bufio.Writer, framing mcpio.FramingMode, id any, code int, message string) error {
	resp := mcpio.Response{
		JSONRPC: mcpio.JSONRPCVersion,
		ID:      id,
		Error:   &mcpio.RPCError{Code: code, Message: message},
	}
	if err := mcpio.WriteMessage(writer, resp, framing); err != nil {
		return err
	}
	return writer.Flush()
}

func logErr(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "[%s] %v\n", serverName, err)
}
