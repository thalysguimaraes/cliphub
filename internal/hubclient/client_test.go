package hubclient

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestStreamURLAndEndpointConstruction(t *testing.T) {
	var seenPath string
	var seenQuery string

	client, err := New(Config{
		BaseURL: "https://example.com/root/",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				seenPath = req.URL.Path
				seenQuery = req.URL.RawQuery
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       ioNopCloser("[]"),
					Header:     make(http.Header),
				}, nil
			}),
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if got := client.StreamURL(); got != "wss://example.com/root/api/clip/stream" {
		t.Fatalf("unexpected stream URL %q", got)
	}

	if _, err := client.History(context.Background(), 7); err != nil {
		t.Fatalf("History() error = %v", err)
	}
	if seenPath != "/root/api/clip/history" {
		t.Fatalf("unexpected request path %q", seenPath)
	}
	if seenQuery != "limit=7" {
		t.Fatalf("unexpected request query %q", seenQuery)
	}
}

func TestCurrentAppliesRequestTimeout(t *testing.T) {
	client, err := New(Config{
		BaseURL:        "http://example.com",
		RequestTimeout: 25 * time.Millisecond,
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				<-req.Context().Done()
				return nil, req.Context().Err()
			}),
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Current(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestPutReturnsHTTPErrorBody(t *testing.T) {
	client, err := New(Config{
		BaseURL: "http://example.com",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       ioNopCloser("empty content"),
					Header:     make(http.Header),
				}, nil
			}),
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Put(context.Background(), PutRequest{MimeType: "text/plain"})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status code %d", httpErr.StatusCode)
	}
	if !strings.Contains(httpErr.Error(), "empty content") {
		t.Fatalf("expected response body in error, got %q", httpErr.Error())
	}
}

func TestClearUsesDeleteEndpoint(t *testing.T) {
	var seenMethod string
	var seenPath string

	client, err := New(Config{
		BaseURL: "http://example.com/root",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				seenMethod = req.Method
				seenPath = req.URL.Path
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       http.NoBody,
					Header:     make(http.Header),
				}, nil
			}),
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := client.Clear(context.Background()); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if seenMethod != http.MethodDelete {
		t.Fatalf("expected DELETE, got %s", seenMethod)
	}
	if seenPath != "/root/api/clip" {
		t.Fatalf("unexpected request path %q", seenPath)
	}
}

func TestCurrentNoContent(t *testing.T) {
	client, err := New(Config{
		BaseURL: "http://example.com",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       http.NoBody,
					Header:     make(http.Header),
				}, nil
			}),
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Current(context.Background())
	if !errors.Is(err, ErrNoCurrentClip) {
		t.Fatalf("expected ErrNoCurrentClip, got %v", err)
	}
}

func ioNopCloser(body string) *readCloser {
	return &readCloser{Reader: strings.NewReader(body)}
}

type readCloser struct {
	*strings.Reader
}

func (rc *readCloser) Close() error { return nil }
