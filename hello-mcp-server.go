// hello-mcp-server.go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
)

// 定义通用的请求和响应结构体，以匹配MCP的JSON-RPC格式
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	// MCP服务器通过标准输入/输出进行通信，所以我们需要一个扫描器来读取stdin
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()

		// 读取每一行（通常是一个JSON-RPC请求），并尝试解析
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			log.Printf("Error unmarshaling request: %v", err)
			continue
		}

		// 根据请求的Method字段，路由到不同的处理函数
		switch req.Method {
		case "initialize":
			handleInitialize(req)
		case "tools/list":
			handleToolsList(req)
		case "tools/call":
			handleToolCall(req)
		case "notifications/initialized":
			// 客户端发送的初始化完成通知，无需响应
			continue
		default:
			sendError(req.ID, -32601, "Method not found")
		}
	}
}

// handleInitialize负责向Claude Code"自我介绍"
func handleInitialize(req Request) {
	// 符合MCP协议的initialize响应
	initializeResult := map[string]any{
		"protocolVersion": "2024-11-05", // MCP协议版本
		"capabilities": map[string]any{
			"tools": map[string]any{}, // 声明支持工具能力
		},
		"serverInfo": map[string]any{
			"name":    "hello-server",
			"version": "1.0.0",
		},
	}
	sendResult(req.ID, initializeResult)
}

// handleToolsList返回可用工具列表
func handleToolsList(req Request) {
	toolsListResult := map[string]any{
		"tools": []map[string]any{
			{
				"name":        "greet",
				"description": "A simple tool that returns a greeting.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "The name of the person to greet.",
						},
					},
					"required": []string{"name"},
				},
			},
		},
	}
	sendResult(req.ID, toolsListResult)
}

// handleToolCall负责处理工具的实际调用
func handleToolCall(req Request) {
	var params map[string]any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendError(req.ID, -32602, "Invalid params")
		return
	}

	toolName, _ := params["name"].(string)
	if toolName != "greet" {
		sendError(req.ID, -32601, "Tool not found")
		return
	}

	toolArguments, _ := params["arguments"].(map[string]any)
	name, _ := toolArguments["name"].(string)

	// 这是我们工具的核心逻辑
	greeting := fmt.Sprintf("Hello, %s! Welcome to the world of MCP in Go.", name)

	// MCP期望的响应格式
	toolResult := map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": greeting,
			},
		},
	}
	sendResult(req.ID, toolResult)
}

// sendResult和sendError是辅助函数，用于向stdout发送格式化的JSON-RPC响应
func sendResult(id any, result any) {
	resp := Response{JSONRPC: "2.0", ID: id, Result: result}
	sendJSON(resp)
}

func sendError(id any, code int, message string) {
	resp := Response{JSONRPC: "2.0", ID: id, Error: &Error{Code: code, Message: message}}
	sendJSON(resp)
}

func sendJSON(v any) {
	encoded, err := json.Marshal(v)
	if err != nil {
		log.Printf("Error marshaling response: %v", err)
		return
	}
	// MCP协议要求每个JSON对象后都有一个换行符
	fmt.Println(string(encoded))
}
