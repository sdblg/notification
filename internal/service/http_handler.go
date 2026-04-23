package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/sod/notification/internal/provider"
)

type NotifyHandler struct {
	pool *WorkerPool
	from string
}

type NotifyRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html,omitempty"`
	Text    string `json:"text,omitempty"`
}

type NotifyResponse struct {
	Status    string `json:"status"`
	RequestID string `json:"request_id,omitempty"`
	Message   string `json:"message,omitempty"`
}

func NewNotifyHandler(pool *WorkerPool, from string) *NotifyHandler {
	return &NotifyHandler{
		pool: pool,
		from: from,
	}
}

func (h *NotifyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, NotifyResponse{
			Status:  "error",
			Message: "method not allowed",
		})
		return
	}

	var req NotifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, NotifyResponse{
			Status:  "error",
			Message: "invalid json payload",
		})
		return
	}

	req.To = strings.TrimSpace(req.To)
	req.Subject = strings.TrimSpace(req.Subject)

	notif := provider.EmailNotification{
		To:      req.To,
		Subject: req.Subject,
		HTML:    req.HTML,
		Text:    req.Text,
	}
	if err := notif.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, NotifyResponse{
			Status:  "error",
			Message: err.Error(),
		})
		return
	}

	msg := provider.EmailMessage{
		From:    h.from,
		To:      req.To,
		Subject: req.Subject,
		HTML:    req.HTML,
		Text:    req.Text,
		Headers: map[string]string{
			"Reply-To":                 h.from,
			"List-Unsubscribe":         "<mailto:unsubscribe@example.com>",
			"X-Auto-Response-Suppress": "OOF, AutoReply",
			"Precedence":               "bulk",
		},
	}

	requestID, err := h.pool.Enqueue(r.Context(), msg)
	if err != nil {
		statusCode := http.StatusInternalServerError
		message := "failed to enqueue notification"
		if errors.Is(err, ErrQueueFull) {
			statusCode = http.StatusTooManyRequests
			message = "notification queue is full"
		}
		writeJSON(w, statusCode, NotifyResponse{
			Status:  "error",
			Message: message,
		})
		return
	}

	writeJSON(w, http.StatusAccepted, NotifyResponse{
		Status:    "accepted",
		RequestID: requestID,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload NotifyResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
