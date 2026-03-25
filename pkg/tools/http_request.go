package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/constants"
)

type HTTPRequestTool struct{}

func NewHTTPRequestTool() *HTTPRequestTool {
	return &HTTPRequestTool{}
}

func (t *HTTPRequestTool) Name() string {
	return "http_request"
}

func (t *HTTPRequestTool) Description() string {
	return "Make an HTTP request (GET, POST, PUT, PATCH, DELETE) with custom headers and body. Use this to call APIs, webhooks, or any REST endpoint."
}

func (t *HTTPRequestTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"method": map[string]any{
				"type":        "string",
				"enum":        []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
				"description": "HTTP method",
			},
			"url": map[string]any{
				"type":        "string",
				"description": "The URL to send the request to",
			},
			"headers": map[string]any{
				"type":        "object",
				"description": "Optional HTTP headers as key-value pairs",
			},
			"body": map[string]any{
				"type":        "string",
				"description": "Optional request body (use JSON string for JSON APIs)",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Request timeout in seconds (default: 30)",
				"default":     30,
			},
		},
		"required": []string{"method", "url"},
	}
}

func (t *HTTPRequestTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if !constants.IsInternalChannel(ToolChannel(ctx)) {
		return ErrorResult("http_request is restricted to internal channels")
	}
	method, ok := args["method"].(string)
	if !ok {
		return ErrorResult("method is required")
	}
	urlStr, ok := args["url"].(string)
	if !ok || urlStr == "" {
		return ErrorResult("url is required")
	}

	timeout := 30 * time.Second
	if ts, ok := args["timeout_seconds"].(float64); ok && ts > 0 {
		timeout = time.Duration(ts) * time.Second
	}

	var bodyReader io.Reader
	if body, ok := args["body"].(string); ok && body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), urlStr, bodyReader)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create request: %v", err))
	}

	if headers, ok := args["headers"].(map[string]any); ok {
		for k, v := range headers {
			if vs, ok := v.(string); ok {
				req.Header.Set(k, vs)
			}
		}
	}

	// Default Content-Type for requests with a body
	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("request failed: %v", err))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read response: %v", err))
	}

	// Pretty-print JSON responses
	bodyStr := string(respBody)
	if strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		var pretty bytes.Buffer
		if json.Indent(&pretty, respBody, "", "  ") == nil {
			bodyStr = pretty.String()
		}
	}

	result := fmt.Sprintf("Status: %d %s\n\n%s", resp.StatusCode, resp.Status, bodyStr)

	if resp.StatusCode >= 400 {
		return &ToolResult{ForLLM: result, ForUser: result, IsError: true}
	}
	return UserResult(result)
}
