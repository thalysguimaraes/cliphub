package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/thalys/cliphub/internal/protocol"
)

// WSClient connects to the hub's WebSocket stream and delivers updates.
type WSClient struct {
	URL      string
	OnUpdate func(protocol.ClipItem)
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
			slog.Warn("websocket disconnected", "err", err, "retry_in", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 60*time.Second)
			continue
		}
		backoff = time.Second // Reset on successful connection.
	}
}

func (c *WSClient) connect(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, c.URL, nil)
	if err != nil {
		return err
	}
	defer conn.CloseNow()

	slog.Info("connected to hub", "url", c.URL)

	for {
		var msg protocol.WSMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return err
		}
		if msg.Type == "clip_update" && msg.Item != nil {
			c.OnUpdate(*msg.Item)
		}
	}
}
