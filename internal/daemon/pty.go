package daemon

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type ptyMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

func jsonString(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// handlePTY bridges a session's terminal to a WebSocket. Output is sent as binary
// frames; input and resize events arrive as JSON text frames.
func (s *Server) handlePTY(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.cfg.Sessions.Get(r.PathValue("id"))
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	upgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 32 * 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			return origin == "" || s.allowedOrigin(origin)
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1 << 20)

	hist, ch, cancel := sess.Subscribe()
	defer cancel()
	if len(hist) > 0 {
		if err := conn.WriteMessage(websocket.BinaryMessage, hist); err != nil {
			return
		}
	}

	go func() {
		for {
			var msg ptyMessage
			if err := conn.ReadJSON(&msg); err != nil {
				cancel()
				return
			}
			switch msg.Type {
			case "input":
				_, _ = sess.Write([]byte(msg.Data))
			case "resize":
				if msg.Cols > 0 && msg.Rows > 0 {
					_ = sess.Resize(msg.Cols, msg.Rows)
				}
			}
		}
	}()

	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				info := sess.Info()
				_ = conn.WriteJSON(map[string]any{"type": "exit", "session": info})
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session ended"), time.Now().Add(time.Second))
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.BinaryMessage, chunk); err != nil {
				return
			}
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}
