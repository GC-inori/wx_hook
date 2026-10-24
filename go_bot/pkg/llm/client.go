package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"wxbot/pkg/config"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type Client struct {
	cfg    config.LLMConfig
	client *http.Client
}

func NewClient(cfg config.LLMConfig) *Client {
	return &Client{
		cfg: cfg,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *Client) IsConfigured() bool {
	return c.cfg.APIKey != "" && c.cfg.BaseURL != "" && c.cfg.Model != ""
}

func (c *Client) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if !c.IsConfigured() {
		return "", fmt.Errorf("LLM 尚未配置 APIKey 或 BaseURL")
	}

	messages := []ChatMessage{}
	if systemPrompt != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, ChatMessage{Role: "user", Content: userPrompt})

	reqBody := ChatRequest{
		Model:       c.cfg.Model,
		Messages:    messages,
		Temperature: 0.7,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("请求 LLM 接口失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", fmt.Errorf("解析响应 JSON 失败: %w", err)
	}

	if chatResp.Error != nil && chatResp.Error.Message != "" {
		return "", fmt.Errorf("LLM 返回错误: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("LLM 未返回有效回复内容")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}
