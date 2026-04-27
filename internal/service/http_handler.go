package service

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/sdblg/notification/internal/provider"
	"github.com/sdblg/notification/pkg/models"
)

type NotifyHandler struct {
	pool   *WorkerPool
	from   string
	logger *slog.Logger
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
	TraceID   string `json:"trace_id,omitempty"`
	Message   string `json:"message,omitempty"`
}

func NewNotifyHandler(pool *WorkerPool, from string, logger *slog.Logger) *NotifyHandler {
	return &NotifyHandler{
		pool:   pool,
		from:   from,
		logger: logger,
	}
}

func (h *NotifyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := LoggerForRequest(r, h.logger, "notify_handler")

	if r.Method != http.MethodPost {
		logger.Warn("method not allowed", "method", r.Method, "path", r.URL.Path)
		writeJSON(w, r, http.StatusMethodNotAllowed, NotifyResponse{
			Status:  "error",
			Message: "method not allowed",
		})
		return
	}

	var req NotifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("invalid json payload", "path", r.URL.Path, "error", err)
		writeJSON(w, r, http.StatusBadRequest, NotifyResponse{
			Status:  "error",
			Message: "invalid json payload",
		})
		return
	}

	req.To = strings.TrimSpace(req.To)
	req.Subject = strings.TrimSpace(req.Subject)

	body := models.EmailBody{Plain: req.Text, Rich: req.HTML}
	notif := provider.EmailNotification{
		To:      req.To,
		Subject: req.Subject,
		Body:    body,
	}
	if err := notif.Validate(); err != nil {
		logger.Warn("validation failed", "to", req.To, "subject", req.Subject, "error", err)
		writeJSON(w, r, http.StatusBadRequest, NotifyResponse{
			Status:  "error",
			Message: err.Error(),
		})
		return
	}

	msg := models.EmailMessage{
		From:    h.from,
		To:      req.To,
		Subject: req.Subject,
		Body:    body,
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
		logger.Error("enqueue failed", "to", req.To, "subject", req.Subject, "status_code", statusCode, "error", err)
		writeJSON(w, r, statusCode, NotifyResponse{
			Status:  "error",
			Message: message,
		})
		return
	}

	logger.Info("notification accepted", "request_id", requestID, "to", req.To)
	writeJSON(w, r, http.StatusAccepted, NotifyResponse{
		Status:    "accepted",
		RequestID: requestID,
	})
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, payload NotifyResponse) {
	if id := TraceIDFromContext(r.Context()); id != "" {
		payload.TraceID = id
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
