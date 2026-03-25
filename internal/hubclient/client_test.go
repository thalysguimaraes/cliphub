package hubclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/protocol"
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

func TestPutBinaryUsesBlobEndpoint(t *testing.T) {
	var (
		seenPath        string
		seenContentType string
		seenBody        []byte
	)

	client, err := New(Config{
		BaseURL: "http://example.com",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				seenPath = req.URL.Path
				seenContentType = req.Header.Get("Content-Type")
				seenBody, _ = io.ReadAll(req.Body)

				payload, _ := json.Marshal(protocol.ClipSummary{
					Seq:       7,
					MimeType:  "image/png",
					Hash:      "abc",
					Source:    "node1",
					CreatedAt: time.Unix(10, 0).UTC(),
					ExpiresAt: time.Unix(20, 0).UTC(),
				})
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       ioNopCloser(string(payload)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	item, err := client.Put(context.Background(), PutRequest{MimeType: "image/png", Data: []byte{1, 2, 3}})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if seenPath != "/api/clip/blob" {
		t.Fatalf("expected blob endpoint, got %q", seenPath)
	}
	if seenContentType != "image/png" {
		t.Fatalf("expected raw content-type, got %q", seenContentType)
	}
	if string(seenBody) != string([]byte{1, 2, 3}) {
		t.Fatalf("expected raw body, got %#v", seenBody)
	}
	if item.Seq != 7 || len(item.Data) != 3 {
		t.Fatalf("unexpected normalized item %+v", item)
	}
}

func TestHistoryPageBuildsQueryAndDecodesEnvelope(t *testing.T) {
	var seenPath string
	var seenQuery string

	payload, _ := json.Marshal(protocol.HistoryPage{
		Items: []protocol.ClipSummary{{Seq: 5, MimeType: "text/plain", Content: "hello"}},
	})
	client, err := New(Config{
		BaseURL: "http://example.com/root",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				seenPath = req.URL.Path
				seenQuery = req.URL.RawQuery
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       ioNopCloser(string(payload)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	page, err := client.HistoryPage(context.Background(), 25, "99")
	if err != nil {
		t.Fatalf("HistoryPage() error = %v", err)
	}
	if seenPath != "/root/api/clip/history/page" {
		t.Fatalf("unexpected path %q", seenPath)
	}
	if seenQuery != "cursor=99&limit=25" && seenQuery != "limit=25&cursor=99" {
		t.Fatalf("unexpected query %q", seenQuery)
	}
	if len(page.Items) != 1 || page.Items[0].Seq != 5 {
		t.Fatalf("unexpected page %+v", page)
	}
}

func TestDownloadCurrentBlob(t *testing.T) {
	client, err := New(Config{
		BaseURL: "http://example.com",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				header := make(http.Header)
				header.Set("Content-Type", "image/png")
				header.Set("X-Clip-Seq", "12")
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       ioNopCloser("raw-bytes"),
					Header:     header,
				}, nil
			}),
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	blob, err := client.Download(context.Background(), 0)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if blob.MimeType != "image/png" || blob.Seq != 12 || string(blob.Data) != "raw-bytes" {
		t.Fatalf("unexpected blob %+v", blob)
	}
}

func TestStructuredHTTPErrorParsing(t *testing.T) {
	payload, _ := json.Marshal(protocol.ErrorResponse{
		Error: protocol.APIError{
			Code:    "invalid_limit",
			Message: "limit must be a positive integer",
			Details: map[string]string{"limit": "abc"},
		},
	})
	client, err := New(Config{
		BaseURL: "http://example.com",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       ioNopCloser(string(payload)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.HistoryPage(context.Background(), 50, "abc")
	if err == nil {
		t.Fatal("expected structured HTTP error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if httpErr.Code != "invalid_limit" || httpErr.Details["limit"] != "abc" {
		t.Fatalf("unexpected structured error %+v", httpErr)
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
