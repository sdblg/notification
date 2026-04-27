package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sdblg/notification/pkg/models"
	"github.com/sony/gobreaker"
)

const (
	notifyPath    = "/v1/notify"
	traceIDHeader = "X-Trace-Id"
)

// Client sends notifications to the notification HTTP API.
type Client struct {
	BaseURL   string
	APIKey    string
	// TraceID, if set, is sent as X-Trace-Id. If empty, a new UUID is used per request.
	TraceID string
	http    *http.Client
	breaker *gobreaker.CircuitBreaker
}

// NewClient creates an SDK client with API key auth and a circuit breaker.
//
// The client uses:
//   - 10s HTTP timeout
//   - circuit breaker trips after 5 consecutive failures
//   - circuit remains open for 30s before trying half-open requests
func NewClient(baseURL, apiKey string) *Client {
	settings := gobreaker.Settings{
		Name:    "notification-send",
		Timeout: 30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	}

	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:  strings.TrimSpace(apiKey),
		http:    &http.Client{Timeout: 10 * time.Second},
		breaker: gobreaker.NewCircuitBreaker(settings),
	}
}

// notifyWire matches the current /v1/notify API payload.
type notifyWire struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html,omitempty"`
	Text    string `json:"text,omitempty"`
}

// Send enqueues an email via POST /v1/notify, wrapped by the circuit breaker.
//
// The server applies From and default MIME headers; msg.From and msg.Headers are
// not sent on this endpoint today.
func (c *Client) Send(msg models.EmailMessage) error {
	_, err := c.breaker.Execute(func() (any, error) {
		return nil, c.sendEmail(msg)
	})
	return err
}

func (c *Client) sendEmail(msg models.EmailMessage) error {
	traceID := strings.TrimSpace(c.TraceID)
	if traceID == "" {
		traceID = uuid.NewString()
	}

	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("sdk: baseURL is required: trace_id=%s", traceID)
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("sdk: apiKey is required: trace_id=%s", traceID)
	}

	if strings.TrimSpace(msg.To) == "" {
		return fmt.Errorf("sdk: message.to is required: trace_id=%s", traceID)
	}
	if strings.TrimSpace(msg.Subject) == "" {
		return fmt.Errorf("sdk: message.subject is required: trace_id=%s", traceID)
	}
	if msg.Body.Plain == "" && msg.Body.Rich == "" {
		return fmt.Errorf("sdk: message body must set plain and/or rich content: trace_id=%s", traceID)
	}

	u, err := url.Parse(c.BaseURL + notifyPath)
	if err != nil {
		return fmt.Errorf("sdk: parse url: trace_id=%s err=%w", traceID, err)
	}

	payload := notifyWire{
		To:      strings.TrimSpace(msg.To),
		Subject: strings.TrimSpace(msg.Subject),
		HTML:    msg.Body.Rich,
		Text:    msg.Body.Plain,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("sdk: marshal request: trace_id=%s err=%w", traceID, err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sdk: build request: trace_id=%s err=%w", traceID, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set(traceIDHeader, traceID)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sdk: http: trace_id=%s err=%w", traceID, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("sdk: read body: trace_id=%s err=%w", traceID, err)
	}

	switch resp.StatusCode {
	case http.StatusAccepted:
		return nil
	default:
		return fmt.Errorf("sdk: request failed: trace_id=%s status=%d body=%s", traceID, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
}
