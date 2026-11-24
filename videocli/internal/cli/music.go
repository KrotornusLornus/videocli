package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"videocli/internal/mpvipc"
)

// ipcSocketPath devuelve la ruta del socket IPC de mpv (XDG_RUNTIME_DIR).
// NO toca el fichero: el socket de un mpv vivo debe seguir ahí.
func ipcSocketPath() (string, error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		return "", fmt.Errorf("XDG_RUNTIME_DIR no está definido")
	}
	_ = os.MkdirAll(filepath.Join(base, "videocli"), 0o700)
	return filepath.Join(base, "videocli", "mpv.sock"), nil
}

// ipcSocket es la versión para LANZAR mpv: limpia el socket previo de un
// reproductor ya muerto.
func ipcSocket() (string, error) {
	sock, err := ipcSocketPath()
	if err != nil {
		return "", err
	}
	_ = os.Remove(sock)
	return sock, nil
}

func musicClient() *mpvipc.Client {
	sock, err := ipcSocketPath()
	if err != nil {
		return nil
	}
	return mpvipc.New(sock)
}

// cmdNotify envía una notificación de escritorio si notify.enabled.
func cmdNotify(c *ctx) int {
	if len(c.args) == 0 {
		return fail(2, "uso interno: videocli notify MENSAJE")
	}
	if !c.cfg.Notify.Enabled {
		return 0
	}
	msg := strings.Join(c.args, " ")
	if cmd, err := execLookPath(c.cfg.Notify.Command); err == nil {
		_ = startDetached(cmd, msg)
	}
	return 0
}

// cmdMusic controla el mpv activo por IPC.
func cmdMusic(c *ctx) int {
	if len(c.args) == 0 {
		fmt.Fprint(os.Stderr, `uso: videocli music <acción>

  play    reanudar        pause  pausar
  toggle  pausa/reanudar  stop   cerrar mpv
  next    siguiente       prev   anterior
  vol-up  volumen +5      vol-down  volumen -5
  status [--json]         estado (pista, posición, pausa)
  list [--thumbs]         cola completa (▶ = sonando)
  jump N                  saltar a la pista N (0-based)
`)
		return 2
	}
	action, rest := c.args[0], c.args[1:]
	ctx := context.Background()
	client := musicClient()
	if client == nil {
		return fail(1, "no se pudo determinar el socket IPC")
	}
	switch action {
	case "play":
		return musicRet(musicSet(ctx, client, false))
	case "pause":
		return musicRet(musicSet(ctx, client, true))
	case "toggle":
		return musicRet(musicSet(ctx, client, nil))
	case "next":
		return musicRet(musicCmd(ctx, client, "playlist-next"))
	case "vol-up":
		return musicRet(musicCmd(ctx, client, "add volume 5"))
	case "vol-down":
		return musicRet(musicCmd(ctx, client, "add volume -5"))
	case "prev":
		return musicRet(musicCmd(ctx, client, "playlist-prev"))
	case "stop":
		return musicRet(musicCmd(ctx, client, "quit"))
	case "jump":
		if len(rest) != 1 {
			return fail(2, "uso: videocli music jump N")
		}
		var idx int
		if _, err := fmt.Sscanf(rest[0], "%d", &idx); err != nil {
			return fail(2, "índice no válido: %s", rest[0])
		}
		return musicRet(musicCmd(ctx, client, fmt.Sprintf("playlist-play-index %d", idx)))
	case "status":
		return musicStatus(c, client, containsFlag(rest, "--json"))
	case "list":
		return musicList(c, client, containsFlag(rest, "--thumbs"))
	default:
		return fail(2, "acción desconocida: %s", action)
	}
}

func containsFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// musicRet convierte un error en código de salida (cancelación = ok).
func musicRet(err error) int {
	if err != nil {
		return fail(1, "%v", err)
	}
	return 0
}

func musicCmd(ctx context.Context, client *mpvipc.Client, args string) error {
	parts := strings.Split(args, " ")
	return client.Command(ctx, parts...)
}

// musicSet fija pause=true/false o alterna (nil).
func musicSet(ctx context.Context, client *mpvipc.Client, pause any) error {
	if pause == nil {
		return client.Command(ctx, "cycle", "pause")
	}
	v, _ := pause.(bool)
	flag := "no"
	if v {
		flag = "yes"
	}
	return client.Command(ctx, "set", "pause", flag)
}

func musicStatus(c *ctx, client *mpvipc.Client, asJSON bool) int {
	st, err := client.Status(context.Background())
	if err != nil {
		return fail(1, "%v", err)
	}
	if asJSON {
		data, _ := json.Marshal(st)
		fmt.Println(string(data))
		return 0
	}
	if !st.Running {
		return 0 // cola terminada: sin salida
	}
	pos := formatSeconds(st.Position)
	dur := formatSeconds(st.Duration)
	flag := " "
	if st.Paused {
		flag = "⏸"
	}
	fmt.Printf("%s\t%s\t%s/%s\t%d/%d\n", flag, st.Title, pos, dur, st.Index+1, st.Count)
	return 0
}

// musicList imprime la cola: índice, marca de actual, miniatura (opcional) y título.
func musicList(c *ctx, client *mpvipc.Client, thumbs bool) int {
	tracks, err := client.Playlist(context.Background())
	if err != nil {
		return fail(1, "%v", err)
	}
	for i, t := range tracks {
		mark := " "
		if t.Current {
			mark = "▶"
		}
		thumb := ""
		if thumbs {
			thumb = ""
			if v, err := c.dbr.FindByPath(t.Filename); err == nil {
				thumb = v.ThumbPath
			}
		}
		title := filepath.Base(t.Filename)
		if v, err := c.dbr.FindByPath(t.Filename); err == nil && v.Title != "" {
			title = v.Title
		}
		if thumbs {
			fmt.Printf("%d\t%s\t%s\t%s\n", i, mark, thumb, title)
		} else {
			fmt.Printf("%d\t%s\t%s\n", i, mark, title)
		}
	}
	return 0
}

func formatSeconds(s float64) string {
	if s <= 0 {
		return "0:00"
	}
	total := int(s)
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

// ejec helpers
func execLookPath(name string) (string, error) {
	return lookPath(name)
}
