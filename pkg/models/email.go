package models

// EmailBody is the MIME payload for an email. At least one of Plain or Rich
// should be non-empty before the message is considered valid for delivery.
//
// Plain is text/plain; Rich is text/html. Both may be set for multipart/alternative
// style mail where clients can pick a preferred representation.
type EmailBody struct {
	// Plain is the text/plain body (often used as a fallback when Rich is set).
	Plain string `json:"plain,omitempty"`
	// Rich is the text/html body.
	Rich string `json:"rich,omitempty"`
}

// EmailMessage is a provider-agnostic outbound email. It is the canonical shape
// for queueing and sending; map it from your transport (HTTP, gRPC, events) at the edge.
type EmailMessage struct {
	// From is the RFC5322 From address or "Name <email>" form, depending on provider rules.
	From string `json:"from,omitempty"`
	// To is a single recipient address (extend the model if you need multiple To/CC/BCC).
	To string `json:"to"`
	// Subject is the message subject line.
	Subject string `json:"subject"`
	// Body carries plain and/or HTML content.
	Body EmailBody `json:"body"`
	// Headers are optional extra MIME headers (e.g. Reply-To, List-Unsubscribe).
	Headers map[string]string `json:"headers,omitempty"`
}
