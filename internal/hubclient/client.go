package hubclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

const defaultRequestTimeout = 10 * time.Second

// Config configures a hub client.
type Config struct {
	BaseURL        string
	RequestTimeout time.Duration
	HTTPClient     *http.Client
}

// PutRequest is the JSON payload accepted by the hub.
type PutRequest struct {
	MimeType string `json:"mime_type,omitempty"`
	Content  string `json:"content,omitempty"`
	Data     []byte `json:"data,omitempty"`
	Source   string `json:"-"`
}

// Client wraps all HTTP interactions with the hub.
type Client struct {
	baseURL        *url.URL
	httpClient     *http.Client
	requestTimeout time.Duration
}

// Blob contains raw clip bytes from the scalable blob download endpoint.
type Blob struct {
	Seq      uint64
	MimeType string
	Data     []byte
}

// ErrNoCurrentClip reports the hub's empty clipboard response.
var ErrNoCurrentClip = errors.New("hub has no current clip")

// HTTPError wraps non-success hub responses.
type HTTPError struct {
	StatusCode int
	Code       string
	Message    string
	Details    map[string]string
	Body       string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return "hub request failed"
	}
	if e.Message != "" && e.Code != "" {
		return fmt.Sprintf("hub: %s [%s] (%d)", e.Message, e.Code, e.StatusCode)
	}
	if e.Message != "" {
		return fmt.Sprintf("hub: %s (%d)", e.Message, e.StatusCode)
	}
	if e.Body == "" {
		return fmt.Sprintf("hub returned %d", e.StatusCode)
	}
	return fmt.Sprintf("hub: %s (%d)", strings.TrimSpace(e.Body), e.StatusCode)
}

// New constructs a hub client for a resolved base URL.
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("hub base URL is required")
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = defaultRequestTimeout
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}

	baseURL, err := normalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}

	return &Client{
		baseURL:        baseURL,
		httpClient:     cfg.HTTPClient,
		requestTimeout: cfg.RequestTimeout,
	}, nil
}

// BaseURL returns the canonical base URL used by the client.
func (c *Client) BaseURL() string {
	if c == nil || c.baseURL == nil {
		return ""
	}
	return c.baseURL.String()
}

// StreamURL returns the websocket endpoint for live clip updates.
func (c *Client) StreamURL() string {
	u := *c.baseURL
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}
	u.Path = joinURLPath(u.Path, "api", "clip", "stream")
	u.RawQuery = ""
	return u.String()
}

// Current fetches the current clipboard item.
func (c *Client) Current(ctx context.Context) (*protocol.ClipItem, error) {
	resp, err := c.do(ctx, http.MethodGet, c.endpoint("api", "clip"), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, ErrNoCurrentClip
	}
	if resp.StatusCode >= 400 {
		return nil, readHTTPError(resp)
	}

	var item protocol.ClipItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &item, nil
}

// Put submits clipboard content to the hub.
func (c *Client) Put(ctx context.Context, payload PutRequest) (*protocol.ClipItem, error) {
	if payload.MimeType == "" {
		if len(payload.Data) > 0 {
			payload.MimeType = "application/octet-stream"
		} else {
			payload.MimeType = "text/plain"
		}
	}
	if len(payload.Data) > 0 {
		return c.putBlob(ctx, payload)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	headers := http.Header{"Content-Type": []string{"application/json"}}
	if payload.Source != "" {
		headers.Set("X-Clip-Source", payload.Source)
	}

	resp, err := c.do(ctx, http.MethodPost, c.endpoint("api", "clip"), bytes.NewReader(body), headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, readHTTPError(resp)
	}

	var item protocol.ClipItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &item, nil
}

func (c *Client) putBlob(ctx context.Context, payload PutRequest) (*protocol.ClipItem, error) {
	headers := http.Header{"Content-Type": []string{payload.MimeType}}
	if payload.Source != "" {
		headers.Set("X-Clip-Source", payload.Source)
	}

	resp, err := c.do(ctx, http.MethodPost, c.endpoint("api", "clip", "blob"), bytes.NewReader(payload.Data), headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, readHTTPError(resp)
	}

	var summary protocol.ClipSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	item := protocol.ClipItem{
		Seq:       summary.Seq,
		MimeType:  summary.MimeType,
		Hash:      summary.Hash,
		Source:    summary.Source,
		CreatedAt: summary.CreatedAt,
		ExpiresAt: summary.ExpiresAt,
	}
	if strings.HasPrefix(summary.MimeType, "text/") {
		item.Content = string(payload.Data)
	} else {
		item.Data = append([]byte(nil), payload.Data...)
	}
	return &item, nil
}

// History fetches recent clipboard history.
func (c *Client) History(ctx context.Context, limit int) ([]protocol.ClipItem, error) {
	values := url.Values{}
	if limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}

	resp, err := c.do(ctx, http.MethodGet, c.endpoint("api", "clip", "history"), nil, nil, values)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, readHTTPError(resp)
	}

	var items []protocol.ClipItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return items, nil
}

// HistoryPage fetches a cursor-addressable history page from the scalable history endpoint.
func (c *Client) HistoryPage(ctx context.Context, limit int, cursor string) (*protocol.HistoryPage, error) {
	values := url.Values{}
	if limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	if cursor != "" {
		values.Set("cursor", cursor)
	}

	resp, err := c.do(ctx, http.MethodGet, c.endpoint("api", "clip", "history", "page"), nil, nil, values)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, readHTTPError(resp)
	}

	var page protocol.HistoryPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &page, nil
}

// Download fetches raw clip bytes from the scalable blob endpoint.
// When seq is zero, it downloads the current clip.
func (c *Client) Download(ctx context.Context, seq uint64) (*Blob, error) {
	values := url.Values{}
	if seq > 0 {
		values.Set("seq", fmt.Sprintf("%d", seq))
	}

	resp, err := c.do(ctx, http.MethodGet, c.endpoint("api", "clip", "blob"), nil, nil, values)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, ErrNoCurrentClip
	}
	if resp.StatusCode >= 400 {
		return nil, readHTTPError(resp)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	downloaded := &Blob{
		MimeType: strings.TrimSpace(resp.Header.Get("Content-Type")),
		Data:     data,
	}
	if seqHeader := strings.TrimSpace(resp.Header.Get("X-Clip-Seq")); seqHeader != "" {
		if parsed, err := strconv.ParseUint(seqHeader, 10, 64); err == nil {
			downloaded.Seq = parsed
		}
	}
	return downloaded, nil
}

// Status fetches the current hub status payload.
func (c *Client) Status(ctx context.Context) (map[string]any, error) {
	resp, err := c.do(ctx, http.MethodGet, c.endpoint("api", "status"), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, readHTTPError(resp)
	}

	var status map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return status, nil
}

func (c *Client) do(ctx context.Context, method string, endpoint string, body io.Reader, headers http.Header, values ...url.Values) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.requestTimeout)
		defer cancel()
	}

	if len(values) > 0 && values[0] != nil {
		parsedURL, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		parsedURL.RawQuery = values[0].Encode()
		endpoint = parsedURL.String()
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		for _, entry := range value {
			req.Header.Add(key, entry)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) endpoint(parts ...string) string {
	u := *c.baseURL
	u.Path = joinURLPath(u.Path, parts...)
	u.RawQuery = ""
	return u.String()
}

func normalizeBaseURL(raw string) (*url.URL, error) {
	baseURL, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid hub URL %q", raw)
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/")
	baseURL.RawQuery = ""
	baseURL.Fragment = ""
	return baseURL, nil
}

func joinURLPath(basePath string, parts ...string) string {
	segments := make([]string, 0, len(parts)+1)
	if trimmed := strings.Trim(basePath, "/"); trimmed != "" {
		segments = append(segments, trimmed)
	}
	for _, part := range parts {
		if trimmed := strings.Trim(part, "/"); trimmed != "" {
			segments = append(segments, trimmed)
		}
	}
	if len(segments) == 0 {
		return "/"
	}
	return "/" + strings.Join(segments, "/")
}

func readHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	httpErr := &HTTPError{
		StatusCode: resp.StatusCode,
		Body:       string(body),
	}

	var envelope protocol.ErrorResponse
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error.Code != "" {
		httpErr.Code = envelope.Error.Code
		httpErr.Message = envelope.Error.Message
		httpErr.Details = envelope.Error.Details
	}

	return httpErr
}
