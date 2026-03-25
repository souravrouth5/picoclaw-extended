package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type NotifyTool struct{}

func NewNotifyTool() *NotifyTool {
	return &NotifyTool{}
}

func (t *NotifyTool) Name() string {
	return "notify"
}

func (t *NotifyTool) Description() string {
	return "Send a push notification via ntfy.sh (free, no account needed) or Pushover. Useful for alerting when a long background task completes."
}

func (t *NotifyTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "Notification message body",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "Notification title (optional)",
			},
			"provider": map[string]any{
				"type":        "string",
				"enum":        []string{"ntfy", "pushover"},
				"description": "Notification provider. Default: ntfy",
				"default":     "ntfy",
			},
			"ntfy_topic": map[string]any{
				"type":        "string",
				"description": "ntfy.sh topic name (required for ntfy). Subscribe at https://ntfy.sh/<topic>",
			},
			"pushover_token": map[string]any{
				"type":        "string",
				"description": "Pushover application token (required for pushover)",
			},
			"pushover_user": map[string]any{
				"type":        "string",
				"description": "Pushover user key (required for pushover)",
			},
		},
		"required": []string{"message"},
	}
}

func (t *NotifyTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	message, ok := args["message"].(string)
	if !ok || strings.TrimSpace(message) == "" {
		return ErrorResult("message is required")
	}

	title, _ := args["title"].(string)
	provider, _ := args["provider"].(string)
	if provider == "" {
		provider = "ntfy"
	}

	client := &http.Client{Timeout: 10 * time.Second}

	switch provider {
	case "ntfy":
		topic, ok := args["ntfy_topic"].(string)
		if !ok || strings.TrimSpace(topic) == "" {
			return ErrorResult("ntfy_topic is required for ntfy provider")
		}
		url := fmt.Sprintf("https://ntfy.sh/%s", strings.TrimSpace(topic))
		req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(message))
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to create request: %v", err))
		}
		if title != "" {
			req.Header.Set("Title", title)
		}
		resp, err := client.Do(req)
		if err != nil {
			return ErrorResult(fmt.Sprintf("ntfy request failed: %v", err))
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		if resp.StatusCode >= 400 {
			return ErrorResult(fmt.Sprintf("ntfy returned status %d", resp.StatusCode))
		}
		return SilentResult(fmt.Sprintf("Notification sent via ntfy to topic: %s", topic))

	case "pushover":
		token, ok := args["pushover_token"].(string)
		if !ok || token == "" {
			return ErrorResult("pushover_token is required for pushover provider")
		}
		user, ok := args["pushover_user"].(string)
		if !ok || user == "" {
			return ErrorResult("pushover_user is required for pushover provider")
		}
		body := fmt.Sprintf("token=%s&user=%s&message=%s", token, user, message)
		if title != "" {
			body += "&title=" + title
		}
		req, err := http.NewRequestWithContext(ctx, "POST", "https://api.pushover.net/1/messages.json",
			strings.NewReader(body))
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to create request: %v", err))
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := client.Do(req)
		if err != nil {
			return ErrorResult(fmt.Sprintf("pushover request failed: %v", err))
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		if resp.StatusCode >= 400 {
			return ErrorResult(fmt.Sprintf("pushover returned status %d", resp.StatusCode))
		}
		return SilentResult("Notification sent via Pushover")

	default:
		return ErrorResult(fmt.Sprintf("unknown provider: %s", provider))
	}
}
