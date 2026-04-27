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

	"github.com/sdblg/notification/pkg/models"
)

const defaultResendEndpoint = "https://api.resend.com/emails"

type ResendProvider struct {
	apiKey   string
	endpoint string
	client   *http.Client
}

func NewResendProviderFromEnv() (*ResendProvider, error) {
	apiKey := strings.TrimSpace(os.Getenv("RESEND_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("RESEND_API_KEY is not set")
	}

	endpoint := strings.TrimSpace(os.Getenv("RESEND_ENDPOINT"))
	if endpoint == "" {
		endpoint = defaultResendEndpoint
	}

	return &ResendProvider{
		apiKey:   apiKey,
		endpoint: endpoint,
		client:   &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (r *ResendProvider) GetName() string {
	return "resend"
}

func (r *ResendProvider) Send(ctx context.Context, message models.EmailMessage) error {
	payload := map[string]any{
		"from":    message.From,
		"to":      []string{message.To},
		"subject": message.Subject,
		"headers": message.Headers,
	}
	if message.Body.Rich != "" {
		payload["html"] = message.Body.Rich
	}
	if message.Body.Plain != "" {
		payload["text"] = message.Body.Plain
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("resend marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("resend create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("resend call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	respText := strings.ToLower(string(respBody))

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusTooManyRequests:
		return WrapFailoverError(r.GetName(), ErrRateLimited)
	case strings.Contains(respText, "quota") || strings.Contains(respText, "limit exceeded"):
		return WrapFailoverError(r.GetName(), ErrQuotaReached)
	default:
		return fmt.Errorf("resend send failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
}
