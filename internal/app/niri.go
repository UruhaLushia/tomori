package app

import (
	"encoding/json"
	"net"
	"os"
	"time"
)

// Wayland ignores native window positions; Niri exposes floating moves over IPC.
func (a *app) moveNiriWindow(dx, dy float64) {
	socket := os.Getenv("NIRI_SOCKET")
	if socket == "" || os.Getenv("WAYLAND_DISPLAY") == "" || os.Getenv("GDK_BACKEND") == "x11" || (dx == 0 && dy == 0) {
		return
	}
	title := a.win.Title()
	a.workers.Go(func() {
		var reply struct {
			OK struct {
				Windows []struct {
					ID       uint64 `json:"id"`
					PID      int    `json:"pid"`
					Title    string `json:"title"`
					Floating bool   `json:"is_floating"`
				} `json:"Windows"`
			} `json:"Ok"`
		}
		if niriRequest(socket, "Windows", &reply) != nil {
			return
		}
		for _, window := range reply.OK.Windows {
			if window.PID != os.Getpid() || window.Title != title || !window.Floating {
				continue
			}
			request := map[string]any{"Action": map[string]any{"MoveFloatingWindow": map[string]any{
				"id": window.ID, "x": map[string]float64{"AdjustFixed": dx}, "y": map[string]float64{"AdjustFixed": dy},
			}}}
			var result json.RawMessage
			_ = niriRequest(socket, request, &result)
			return
		}
	})
}

func niriRequest(socket string, request, reply any) error {
	conn, err := net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		return err
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return err
	}
	return json.NewDecoder(conn).Decode(reply)
}
