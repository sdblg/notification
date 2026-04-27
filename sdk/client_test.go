package sdk

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sdblg/notification/pkg/models"
)

func TestClient_SendEmailAccepted(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != notifyPath {
			t.Fatalf("path: got %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization: got %q", got)
		}
		if got := r.Header.Get(traceIDHeader); got == "" {
			t.Fatalf("expected %s header", traceIDHeader)
		}
		b, _ := io.ReadAll(r.Body)
		var got notifyWire
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if got.To != "a@b.co" || got.Subject != "hi" || got.HTML != "<p>x</p>" || got.Text != "x" {
			t.Fatalf("wire: %+v", got)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key")
	err := c.Send(models.EmailMessage{
		To:      "a@b.co",
		Subject: "hi",
		Body:    models.EmailBody{Plain: "x", Rich: "<p>x</p>"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_SendCustomTraceID(t *testing.T) {
	const wantTrace = "custom-trace-123"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(traceIDHeader); got != wantTrace {
			t.Fatalf("X-Trace-Id: want %q got %q", wantTrace, got)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key")
	c.TraceID = wantTrace
	err := c.Send(models.EmailMessage{
		To:      "a@b.co",
		Subject: "hi",
		Body:    models.EmailBody{Plain: "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_ErrorIncludesTraceID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":"error"}`))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key")
	c.TraceID = "err-trace-xyz"
	err := c.Send(models.EmailMessage{
		To:      "a@b.co",
		Subject: "hi",
		Body:    models.EmailBody{Plain: "x"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "err-trace-xyz") {
		t.Fatalf("error should include trace_id: %v", err)
	}
}
