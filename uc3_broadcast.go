package drite

import "encoding/json"

// Only Run may mutate clients or close their outgoing channels.
type Hub struct {
	server     *Server
	clients    map[*Client]struct{}
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
}

func newHub(s *Server) *Hub {
	return &Hub{server: s, clients: make(map[*Client]struct{}), register: make(chan *Client), unregister: make(chan *Client), broadcast: make(chan []byte, 256)}
}
func (h *Hub) remove(c *Client) {
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
		_ = c.conn.Close()
	}
}
func (h *Hub) Run() {
	defer func() {
		for c := range h.clients {
			h.remove(c)
		}
	}()
	for {
		select {
		case <-h.server.ctx.Done():
			return
		case c := <-h.register:
			h.clients[c] = struct{}{}
		case c := <-h.unregister:
			h.remove(c)
		case event := <-h.broadcast:
			for c := range h.clients {
				if !c.dashboard {
					continue
				}
				select {
				case c.send <- event:
				default:
					h.remove(c)
				}
			}
		}
	}
}
func (s *Server) publish(e BroadcastEnvelope) bool {
	data, err := json.Marshal(e)
	if err != nil {
		return false
	}
	select {
	case s.hub.broadcast <- data:
		return true
	case <-s.ctx.Done():
		return false
	}
}
