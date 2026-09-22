package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpenAICompatibleExtractor uses a portable chat-completions contract. Provider
// presets are conveniences only; MODEL_BASE_URL always overrides them.
type OpenAICompatibleExtractor struct {
	Provider string
	BaseURL  string
	Model    string
	APIKey   string
	Client   *http.Client
}

type relayPage struct {
	PageNumber       int    `json:"page_number"`
	Text             string `json:"text"`
	PreviewPNGBase64 string `json:"preview_png_base64,omitempty"`
}
type relayRequest struct {
	CourseTimezone string      `json:"course_timezone"`
	Pages          []relayPage `json:"pages"`
}
type relayCandidate struct {
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	OfficialDueAt    *time.Time `json:"official_due_at"`
	RawDueExpression string     `json:"raw_due_expression"`
	SubmissionMethod string     `json:"submission_method"`
	Evidence         []Evidence `json:"evidence"`
	Uncertain        []string   `json:"uncertain_fields"`
}
type relayResponse struct {
	Candidates []relayCandidate `json:"candidates"`
}

func NewConfiguredExtractor() Extractor {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("MODEL_PROVIDER")))
	apiKey := strings.TrimSpace(os.Getenv("MODEL_API_KEY"))
	if provider == "" || apiKey == "" {
		return RuleExtractor{}
	}
	baseURL, model := strings.TrimRight(strings.TrimSpace(os.Getenv("MODEL_BASE_URL")), "/"), strings.TrimSpace(os.Getenv("MODEL_NAME"))
	if baseURL == "" {
		switch provider {
		case "openai":
			baseURL, model = "https://api.openai.com/v1", defaultModel(model, "gpt-4o-mini")
		case "deepseek":
			baseURL, model = "https://api.deepseek.com", defaultModel(model, "deepseek-chat")
		case "qwen", "dashscope":
			baseURL, model = "https://dashscope.aliyuncs.com/compatible-mode/v1", defaultModel(model, "qwen-plus")
		default:
			return RuleExtractor{}
		}
	}
	if model == "" {
		return RuleExtractor{}
	}
	return OpenAICompatibleExtractor{Provider: provider, BaseURL: baseURL, Model: model, APIKey: apiKey, Client: &http.Client{Timeout: 45 * time.Second}}
}
func defaultModel(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (e OpenAICompatibleExtractor) Extract(ctx context.Context, input ExtractionInput) ([]Extracted, error) {
	pages := make([]map[string]any, 0, len(input.Pages))
	for _, p := range input.Pages {
		pages = append(pages, map[string]any{"page_number": p.Number, "text": p.Text})
	}
	payload, err := json.Marshal(map[string]any{"course_timezone": input.CourseTimezone, "pages": pages})
	if err != nil {
		return nil, err
	}
	prompt := "从给定课程页面中抽取作业候选。只返回 JSON：{\"candidates\":[{\"title\":string,\"description\":string,\"official_due_at\":RFC3339或null,\"raw_due_expression\":string,\"submission_method\":string,\"evidence\":[{\"page_number\":number,\"text_excerpt\":string}],\"uncertain_fields\":[string]}]}。证据片段必须原样来自页面；不确定字段必须标出，不得猜测。\n\n" + string(payload)
	body, err := json.Marshal(map[string]any{"model": e.Model, "messages": []map[string]string{{"role": "system", "content": "你是课程作业信息抽取器。"}, {"role": "user", "content": prompt}}, "temperature": 0})
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(e.BaseURL, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s model API returned %s", e.Provider, resp.Status)
	}
	var response map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&response); err != nil {
		return nil, err
	}
	choices, ok := response["choices"].([]any)
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("%s model API returned no choices", e.Provider)
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("model response choice is invalid")
	}
	message, ok := choice["message"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("model response message is invalid")
	}
	content, ok := message["content"].(string)
	if !ok {
		return nil, fmt.Errorf("model response content is invalid")
	}
	content = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(content), "\x60\x60\x60json"), "\x60\x60\x60"))
	var decoded relayResponse
	if err := json.Unmarshal([]byte(content), &decoded); err != nil {
		return nil, fmt.Errorf("model result is not valid candidate JSON: %w", err)
	}
	result := make([]Extracted, 0, len(decoded.Candidates))
	for _, c := range decoded.Candidates {
		result = append(result, Extracted{Title: c.Title, Description: c.Description, OfficialDueAt: c.OfficialDueAt, RawDueExpression: c.RawDueExpression, SubmissionMethod: c.SubmissionMethod, Evidence: c.Evidence, Uncertain: c.Uncertain})
	}
	return result, nil
}
