package mpvipc

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// Servidor falso que responde como mpv: get_property y playlist.
func TestClientAgainstFakeSocket(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "mpv.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleFake(conn)
		}
	}()

	c := New(sock)
	ctx := context.Background()

	if err := c.Command(ctx, "toggle_pause"); err != nil {
		t.Fatalf("Command: %v", err)
	}
	var paused bool
	if err := c.GetProperty(ctx, "pause", &paused); err != nil || !paused {
		t.Fatalf("GetProperty pause: %v %v", paused, err)
	}
	tracks, err := c.Playlist(ctx)
	if err != nil || len(tracks) != 2 || !tracks[0].Current {
		t.Fatalf("Playlist: %+v %v", tracks, err)
	}
	st, err := c.Status(ctx)
	if err != nil || !st.Running || st.Title != "Fake" || st.Index != 0 || st.Count != 2 {
		t.Fatalf("Status: %+v %v", st, err)
	}
	_ = os.Remove(sock)
}

func handleFake(conn net.Conn) {
	defer conn.Close()
	sc := json.NewDecoder(conn)
	for {
		var req struct {
			Command []any `json:"command"`
		}
		if err := sc.Decode(&req); err != nil {
			return
		}
		resp := map[string]any{"error": "success"}
		if len(req.Command) >= 2 && req.Command[0] == "get_property" {
			switch req.Command[1] {
			case "path":
				resp["data"] = "/fake.mp3"
			case "pause":
				resp["data"] = true
			case "media-title":
				resp["data"] = "Fake"
			case "time-pos":
				resp["data"] = 12.5
			case "duration":
				resp["data"] = 100
			case "playlist":
				resp["data"] = []map[string]any{
					{"filename": "/a.mp3", "current": true},
					{"filename": "/b.mp3"},
				}
			}
		}
		_ = json.NewEncoder(conn).Encode(resp)
	}
}

// Sin socket: Status devuelve Running=false sin error.
func TestStatusWhenMpvGone(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "no-existe.sock"))
	st, err := c.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Running {
		t.Fatal("debería informar Running=false")
	}
}
