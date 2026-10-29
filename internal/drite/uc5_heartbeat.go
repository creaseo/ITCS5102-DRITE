package drite

import (
	"github.com/gorilla/websocket"
	"time"
)

type Client struct {
	server    *Server
	conn      *websocket.Conn
	agentID   string
	dashboard bool
	send      chan []byte
}

func (c *Client) prepareHeartbeat() {
	c.conn.SetReadLimit(c.server.config.ReadLimitByte)
	_ = c.conn.SetReadDeadline(time.Now().Add(c.server.config.PongWait))
	c.conn.SetPongHandler(func(string) error { return c.conn.SetReadDeadline(time.Now().Add(c.server.config.PongWait)) })
}
func (c *Client) disconnect() {
	_ = c.conn.Close()
	select {
	case c.server.hub.unregister <- c:
	case <-c.server.ctx.Done():
	}
}
func (c *Client) policyClose(reason string) {
	// WriteControl is safe alongside the sole application-data writer.
	if len(reason) > 120 {
		reason = reason[:120]
	}
	_ = c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, reason), time.Now().Add(c.server.config.WriteWait))
}
func (c *Client) writePump() {
	ticker := time.NewTicker(c.server.config.PingInterval)
	defer ticker.Stop()
	defer c.disconnect()
	for {
		select {
		case <-c.server.ctx.Done():
			return
		case data, ok := <-c.send:
			if !ok {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(c.server.config.WriteWait))
			if c.conn.WriteMessage(websocket.TextMessage, data) != nil {
				return
			}
		case <-ticker.C:
			if c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(c.server.config.WriteWait)) != nil {
				return
			}
		}
	}
}
