package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/sdblg/notification/internal/provider"
	"github.com/sdblg/notification/pkg/models"
)

type FailoverEmailService struct {
	providers []provider.EmailProvider
}

func NewFailoverEmailService(providers ...provider.EmailProvider) (*FailoverEmailService, error) {
	filtered := make([]provider.EmailProvider, 0, len(providers))
	for _, p := range providers {
		if p != nil {
			filtered = append(filtered, p)
		}
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("at least one provider is required")
	}

	return &FailoverEmailService{providers: filtered}, nil
}

func (s *FailoverEmailService) Send(ctx context.Context, msg models.EmailMessage) error {
	var failures []string

	for _, p := range s.providers {
		err := p.Send(ctx, msg)
		if err == nil {
			return nil
		}

		if provider.IsFailoverError(err) {
			failures = append(failures, fmt.Sprintf("%s(%v)", p.GetName(), err))
			continue
		}

		return fmt.Errorf("provider %s hard failed: %w", p.GetName(), err)
	}

	if len(failures) == 0 {
		return fmt.Errorf("all providers failed without failover context")
	}

	return fmt.Errorf("all providers exhausted: %s", strings.Join(failures, " -> "))
}
