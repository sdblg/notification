package provider

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

const defaultBrevoEndpoint = "https://api.brevo.com/v3/smtp/email"

type BrevoProvider struct {
	apiKey     string
	endpoint   string
	senderName string
	client     *http.Client
}

func NewBrevoProviderFromEnv() (*BrevoProvider, error) {
	apiKey := strings.TrimSpace(os.Getenv("BREVO_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("BREVO_API_KEY is not set")
	}

	endpoint := strings.TrimSpace(os.Getenv("BREVO_ENDPOINT"))
	if endpoint == "" {
		endpoint = defaultBrevoEndpoint
	}

	senderName := strings.TrimSpace(os.Getenv("MAIL_FROM_NAME"))
	if senderName == "" {
		senderName = "Notification Service"
	}

	return &BrevoProvider{
		apiKey:     apiKey,
		endpoint:   endpoint,
		senderName: senderName,
		client:     &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (b *BrevoProvider) GetName() string {
	return "brevo"
}

func (b *BrevoProvider) Send(ctx context.Context, message EmailMessage) error {
	payload := map[string]any{
		"sender": map[string]string{
			"name":  b.senderName,
			"email": message.From,
		},
		"to": []map[string]string{
			{"email": message.To},
		},
		"subject": message.Subject,
		"headers": message.Headers,
	}
	if message.HTML != "" {
		payload["htmlContent"] = message.HTML
	}
	if message.Text != "" {
		payload["textContent"] = message.Text
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("brevo marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("brevo create request: %w", err)
	}

	req.Header.Set("api-key", b.apiKey)
	req.Header.Set("accept", "application/json")
	req.Header.Set("content-type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("brevo call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	respText := strings.ToLower(string(respBody))

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusTooManyRequests:
		return WrapFailoverError(b.GetName(), ErrRateLimited)
	case strings.Contains(respText, "quota") || strings.Contains(respText, "limit exceeded"):
		return WrapFailoverError(b.GetName(), ErrQuotaReached)
	default:
		return fmt.Errorf("brevo send failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
}
