package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type rentFlowOllamaGenerateRequest struct {
	Model       string                 `json:"model"`
	Prompt      string                 `json:"prompt"`
	Stream      bool                   `json:"stream"`
	System      string                 `json:"system,omitempty"`
	Options     map[string]interface{} `json:"options,omitempty"`
	KeepAlive   string                 `json:"keep_alive,omitempty"`
	Format      string                 `json:"format,omitempty"`
	Temperature float64                `json:"temperature,omitempty"`
}

type rentFlowOllamaGenerateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func RentFlowAIBaseURL() string {
	for _, key := range []string{
		"RENTFLOW_AI_OLLAMA_URL",
		"LOCAL_AI_URL",
	} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return strings.TrimRight(value, "/")
		}
	}
	return ""
}

func RentFlowAIModel() string {
	for _, key := range []string{
		"RENTFLOW_AI_MODEL",
		"LOCAL_AI_MODEL",
	} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func RentFlowAIProviderLabel() string {
	model := RentFlowAIModel()
	if model == "" {
		return "database-rules"
	}
	return "ollama:" + model
}

func RentFlowAIEnabled() bool {
	return RentFlowAIBaseURL() != "" && RentFlowAIModel() != ""
}

func RentFlowAIGenerateText(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	baseURL := RentFlowAIBaseURL()
	model := RentFlowAIModel()
	if baseURL == "" || model == "" {
		return "", fmt.Errorf("ai provider not configured")
	}

	requestBody := rentFlowOllamaGenerateRequest{
		Model:     model,
		System:    strings.TrimSpace(systemPrompt),
		Prompt:    strings.TrimSpace(userPrompt),
		Stream:    false,
		KeepAlive: "10m",
		Options: map[string]interface{}{
			"temperature": 0.2,
			"top_p":       0.9,
		},
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	timeout := 20 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
	}

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/generate", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	if token := strings.TrimSpace(os.Getenv("RENTFLOW_AI_BEARER_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var decoded rentFlowOllamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		message := strings.TrimSpace(decoded.Response)
		if message == "" {
			message = resp.Status
		}
		return "", fmt.Errorf("ollama error: %s", message)
	}

	return sanitizeAIText(decoded.Response), nil
}

func sanitizeAIText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"")
	lines := strings.Split(value, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.Join(cleaned, " ")
}
