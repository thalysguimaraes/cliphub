package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

// WSClient connects to the hub's WebSocket stream and delivers updates.
type WSClient struct {
	URL         string
	OnUpdate    func(protocol.ClipItem)
	OnConnected func() // Called after each successful connect.
	lastSeq     uint64 // Tracks last seq received for reconnect catch-up.
}

// Run connects to the hub and reads updates until ctx is cancelled.
// It reconnects automatically with exponential backoff.
func (c *WSClient) Run(ctx context.Context) {
	backoff := time.Second

	for {
		if err := c.connect(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("websocket disconnected", "component", "clipd_stream", "error", err, "retry_delay", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 60*time.Second)
			continue
		}
		backoff = time.Second
	}
}

func (c *WSClient) connect(ctx context.Context) error {
	url := c.URL
	if c.lastSeq > 0 {
		url += fmt.Sprintf("?since_seq=%d", c.lastSeq)
	}

	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return err
	}
	defer conn.CloseNow()

	slog.Info("connected to hub", "component", "clipd_stream", "hub_stream_url", url)

	if c.OnConnected != nil {
		c.OnConnected()
	}

	for {
		var msg protocol.WSMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return err
		}
		if msg.Type == "clip_update" && msg.Item != nil {
			c.lastSeq = msg.Item.Seq
			c.OnUpdate(*msg.Item)
		}
	}
}
