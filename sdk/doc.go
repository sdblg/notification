// Package sdk is the official Go client for the notification HTTP API.
//
// Import this package from other services in the same module or as a module
// dependency (go get github.com/sdblg/notification/sdk) instead of maintaining
// a separate SDK repository.
//
// Send takes pkg/models.EmailMessage for the current email-only /v1/notify API.
// Separate types (e.g. SMS, Slack) exist for future endpoints.
package sdk
