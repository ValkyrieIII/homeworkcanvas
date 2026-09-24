package ingestion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractorUsesPageEvidenceAndNormalizesDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected model request: %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Messages) != 2 || !strings.Contains(body.Messages[1].Content, "实验一报告 10月15日提交") {
			t.Errorf("page evidence missing from request")
		}
		response := map[string]any{"choices": []any{map[string]any{"message": map[string]any{
			"content": `{"candidates":[{"title":"实验一报告","description":"提交报告","official_due_at":"2026-10-15 23:59","raw_due_expression":"10月15日 23:59","evidence":[{"page_number":2,"text_excerpt":"10月15日提交"}],"uncertain_fields":[]}]}`,
		}}}}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	extractor := OpenAICompatibleExtractor{Provider: "custom", BaseURL: server.URL + "/v1", Model: "test-model", APIKey: "test-key", Client: server.Client()}
	items, err := extractor.Extract(context.Background(), ExtractionInput{CurrentDate: "2026-09-23", Pages: []Page{{Number: 2, Text: "实验一报告 10月15日提交"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].OfficialDueAt == nil || items[0].OfficialDueAt.Format("2006-01-02T15:04:05-07:00") != "2026-10-15T23:59:00+08:00" {
		t.Fatalf("unexpected candidates: %+v", items)
	}
}

func TestExtractorReportsProviderErrorWithoutSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_api_key","message":"key rejected"}}`))
	}))
	defer server.Close()
	extractor := OpenAICompatibleExtractor{Provider: "custom", BaseURL: server.URL, Model: "test-model", APIKey: "test-secret", Client: server.Client()}
	_, err := extractor.Extract(context.Background(), ExtractionInput{CurrentDate: "2026-09-23", Pages: []Page{{Number: 1, Text: "作业"}}})
	if err == nil || !strings.Contains(err.Error(), "invalid_api_key") || strings.Contains(err.Error(), "test-secret") {
		t.Fatalf("unexpected provider error: %v", err)
	}
}
