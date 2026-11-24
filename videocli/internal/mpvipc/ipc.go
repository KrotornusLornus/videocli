// Package mpvipc es el cliente del socket IPC JSON de mpv.
package mpvipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Client habla con mpv por su socket UNIX (line-delimited JSON IPC).
type Client struct {
	Socket string
}

// New crea un cliente para el socket indicado.
func New(socket string) *Client { return &Client{Socket: socket} }

// Track es una entrada de la playlist de mpv.
type Track struct {
	Filename string `json:"filename"`
	Current  bool   `json:"current"`
}

// Status es el estado de reproducción que nos interesa.
type Status struct {
	Running  bool    // mpv vivo con el socket accesible
	Path     string  // fichero actual
	Title    string  // media-title
	Position float64 // segundos
	Duration float64 // segundos
	Paused   bool
	Index    int // posición en la cola (0-based), -1 si desconocido
	Count    int // tamaño de la cola
}

// request envía un comando y devuelve el campo "data" de la respuesta,
// saltando cualquier línea que no sea una respuesta.
func (c *Client) request(ctx context.Context, payload map[string]any) (json.RawMessage, error) {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", c.Socket)
	if err != nil {
		return nil, fmt.Errorf("mpv no está en ejecución (%s): %w", c.Socket, err)
	}
	defer conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	_ = conn.SetDeadline(deadline)

	req, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		var resp struct {
			Error string          `json:"error"`
			Data  json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
			continue // línea no JSON (no debería pasar)
		}
		if resp.Error == "" {
			continue // evento, no respuesta
		}
		if resp.Error != "success" {
			return nil, fmt.Errorf("mpv: %s", resp.Error)
		}
		return resp.Data, nil
	}
	return nil, fmt.Errorf("mpv: sin respuesta")
}

// Command envía un comando arbitrario (p. ej. "toggle_pause").
func (c *Client) Command(ctx context.Context, args ...string) error {
	_, err := c.request(ctx, map[string]any{"command": args})
	return err
}

// GetProperty lee una propiedad.
func (c *Client) GetProperty(ctx context.Context, name string, out any) error {
	data, err := c.request(ctx, map[string]any{"command": []string{"get_property", name}})
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// Playlist devuelve la cola actual.
func (c *Client) Playlist(ctx context.Context) ([]Track, error) {
	data, err := c.request(ctx, map[string]any{"command": []string{"get_property", "playlist"}})
	if err != nil {
		return nil, err
	}
	var tracks []Track
	if err := json.Unmarshal(data, &tracks); err != nil {
		return nil, err
	}
	return tracks, nil
}

// Status agrega las propiedades útiles. Si mpv no está corriendo devuelve
// Running=false sin error (fin de cola / reproductor cerrado).
func (c *Client) Status(ctx context.Context) (*Status, error) {
	st := &Status{Running: true, Index: -1}
	get := func(name string, out any) bool {
		return c.GetProperty(ctx, name, out) == nil
	}
	if !get("path", &st.Path) {
		// mpv cerrado o socket muerto.
		return &Status{Running: false}, nil
	}
	_ = get("media-title", &st.Title)
	_ = get("time-pos", &st.Position)
	_ = get("duration", &st.Duration)
	_ = get("pause", &st.Paused)
	var playlist []Track
	if err := c.GetProperty(ctx, "playlist", &playlist); err == nil {
		st.Count = len(playlist)
		for i, t := range playlist {
			if t.Current {
				st.Index = i
				break
			}
		}
	}
	return st, nil
}
