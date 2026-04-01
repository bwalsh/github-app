package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bwalsh/github-app/internal/handler"
)

func TestHandleWebhook_MissingEventHeader(t *testing.T) {
	h := handler.New("test-secret")
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleWebhook_InvalidJSON(t *testing.T) {
	h := handler.New("test-secret")
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`not-json`))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleWebhook_Success(t *testing.T) {
	h := handler.New("test-secret")
	payload := `{"action":"opened","repository":{"full_name":"bwalsh/github-app"},"sender":{"login":"bwalsh"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "pull_request")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "pull_request") {
		t.Errorf("expected response to contain event type, got: %s", body)
	}
	if !strings.Contains(body, "bwalsh/github-app") {
		t.Errorf("expected response to contain repo name, got: %s", body)
	}
}
