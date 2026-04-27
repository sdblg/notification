package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/sdblg/notification/pkg/models"
)

const (
	ChannelEmail = "email"
)

var (
	ErrRateLimited  = errors.New("provider rate limited")
	ErrQuotaReached = errors.New("provider quota reached")
)

// Notification keeps channel-agnostic behavior extensible for future SMS/Slack providers.
type Notification interface {
	Channel() string
	Validate() error
}

type EmailNotification struct {
	To      string
	Subject string
	Body    models.EmailBody
}

func (n EmailNotification) Channel() string {
	return ChannelEmail
}

func (n EmailNotification) Validate() error {
	if n.To == "" {
		return errors.New("to is required")
	}
	if n.Subject == "" {
		return errors.New("subject is required")
	}
	if n.Body.Plain == "" && n.Body.Rich == "" {
		return errors.New("either plain or rich body is required")
	}

	return nil
}

type EmailProvider interface {
	Send(ctx context.Context, message models.EmailMessage) error
	GetName() string
}

func IsFailoverError(err error) bool {
	return errors.Is(err, ErrRateLimited) || errors.Is(err, ErrQuotaReached)
}

func WrapFailoverError(providerName string, err error) error {
	return fmt.Errorf("%s: %w", providerName, err)
}
