// Package handler provides GitHub App webhook event handling.
package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// EventPayload represents a generic GitHub webhook event payload.
type EventPayload struct {
	Action     string `json:"action"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

// Handler handles incoming GitHub App webhook events.
type Handler struct {
	secret string
}

// New creates a new Handler with the given webhook secret.
func New(secret string) *Handler {
	return &Handler{secret: secret}
}

// HandleWebhook processes an incoming webhook HTTP request and returns the parsed event.
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		http.Error(w, "missing X-GitHub-Event header", http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusInternalServerError)
		return
	}

	var payload EventPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "failed to parse payload", http.StatusBadRequest)
		return
	}

	fmt.Fprintf(w, "received %s event: action=%s repo=%s sender=%s\n",
		eventType, payload.Action, payload.Repository.FullName, payload.Sender.Login)
}
