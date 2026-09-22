package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
)

// OpenAICompatibleExtractor 使用通用的 chat-completions 协议。
// 提供商预设只是快捷配置，MODEL_BASE_URL 始终拥有更高优先级。
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
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	OfficialDueAt    json.RawMessage `json:"official_due_at"`
	RawDueExpression string          `json:"raw_due_expression"`
	SubmissionMethod string          `json:"submission_method"`
	Evidence         []Evidence      `json:"evidence"`
	Uncertain        []string        `json:"uncertain_fields"`
}

// rawText 把模型可能写成字符串、数字或 null 的字段取成文本。
func rawText(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "null" {
		return ""
	}
	return strings.Trim(s, `"`)
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
	payload, err := json.Marshal(map[string]any{
		"course_timezone": courseTimezone,
		"current_date":    input.CurrentDate,
		"pages":           pages,
	})
	if err != nil {
		return nil, err
	}
	prompt := "从给定课程页面中抽取作业候选。当前日期是 " + input.CurrentDate +
		"（时区固定 UTC+8）。截止时间写成 UTC+8 的 RFC3339（如 2026-10-01T23:59:00+08:00）；" +
		"页面只给月日没给年份时按当前日期推最近的年份，不要猜成去年。" +
		"只返回 JSON：{\"candidates\":[{\"title\":string," +
		"\"description\":string,\"official_due_at\":RFC3339或null," +
		"\"raw_due_expression\":string,\"submission_method\":string," +
		"\"evidence\":[{\"page_number\":number,\"text_excerpt\":string}]," +
		"\"uncertain_fields\":[string]}]}。证据片段必须原样来自页面；" +
		"不确定字段必须标出，不得猜测。\n\n" + string(payload)
	body, err := json.Marshal(map[string]any{
		"model": e.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是课程作业信息抽取器。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0,
	})
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
		return nil, fmt.Errorf("%s model API returned %s: %s", e.Provider, resp.Status, modelErrorDetail(resp.Body))
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
	return candidatesFromContent(content, input.CurrentDate)
}

// candidatesFromContent 解析模型返回的候选数组，并在这一步把截止时间标准化。
func candidatesFromContent(content, currentDate string) ([]Extracted, error) {
	var envelope struct {
		Candidates []json.RawMessage `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(content), &envelope); err != nil {
		return nil, fmt.Errorf("model result is not valid candidate JSON: %w", err)
	}
	now := chinaNow(currentDate)
	result := make([]Extracted, 0, len(envelope.Candidates))
	for _, raw := range envelope.Candidates {
		var c relayCandidate
		// 逐条解码：某条候选的字段类型不对时只丢这一条，不让整批陪葬。
		if err := json.Unmarshal(raw, &c); err != nil {
			log.Printf("ingestion candidate skipped: %v", err)
			continue
		}
		dueAt := rawText(c.OfficialDueAt)
		if dueAt == "" {
			dueAt = strings.TrimSpace(c.RawDueExpression)
		}
		due := parseDueAt(dueAt, currentDate)
		uncertain := c.Uncertain
		if dueAt != "" && due == nil && !slices.Contains(uncertain, "official_due_at") {
			uncertain = append(uncertain, "official_due_at")
		}
		// 表达式没写年份时，模型给的年份只是猜的，一律按"离当前最近的那一次"重算。
		if due != nil && c.RawDueExpression != "" && !explicitYearPattern.MatchString(c.RawDueExpression) {
			anchored := anchorYear(*due, now)
			due = &anchored
		}
		result = append(result, Extracted{
			Title:            c.Title,
			Description:      c.Description,
			OfficialDueAt:    due,
			RawDueExpression: c.RawDueExpression,
			SubmissionMethod: c.SubmissionMethod,
			Evidence:         c.Evidence,
			Uncertain:        uncertain,
		})
	}
	return result, nil
}

// maxErrorBodyBytes 限制读取错误响应体的长度，避免中转网关回 HTML 错误页时把整页塞进日志和数据库。
const maxErrorBodyBytes = 4 << 10

// modelErrorDetail 从错误响应里取出可直接入库的原因：优先 OpenAI 兼容的 error.code/message，
// 取不到就退回压缩后的原文。
func modelErrorDetail(body io.Reader) string {
	raw, _ := io.ReadAll(io.LimitReader(body, maxErrorBodyBytes))
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Error.Message != "" {
		label := envelope.Error.Code
		if label == "" {
			label = envelope.Error.Type
		}
		if label != "" {
			return truncateOneLine(label+": "+envelope.Error.Message, 300)
		}
		return truncateOneLine(envelope.Error.Message, 300)
	}
	if detail := truncateOneLine(string(raw), 300); detail != "" {
		return detail
	}
	return "(空响应体)"
}

// truncateOneLine 把任意文本压成一行并限长，保证错误原因能安全写进数据库和接口响应。
func truncateOneLine(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return s
}
