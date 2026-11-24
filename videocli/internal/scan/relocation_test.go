package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"videocli/internal/db"
)

// El caso que fallaba: un vídeo descargado (registrado sin hash) se mueve;
// el re-escaneo debe relocalizarlo tras el backfill, no duplicarlo.
func TestScanRelocatesRecordWithoutHash(t *testing.T) {
	cfg, database, tmp := testEnv(t)
	dir := filepath.Join(tmp, "Videos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(dir, "clip.mp4")
	testVideo(t, video)

	// Registrar como lo haría fetch: SIN hash.
	v := &db.Video{Path: video, Title: "clip"}
	if _, err := database.InsertVideo(v); err != nil {
		t.Fatal(err)
	}

	// Mover el fichero y re-escanear.
	moved := filepath.Join(dir, "sub")
	if err := os.MkdirAll(moved, 0o755); err != nil {
		t.Fatal(err)
	}
	movedPath := filepath.Join(moved, "clip.mp4")
	if err := os.Rename(video, movedPath); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), database, cfg, dir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.New != 0 {
		t.Fatalf("el vídeo movido se registró como NUEVO: %+v", res)
	}
	if res.Moved != 1 {
		t.Fatalf("debería relocalizar por hash (backfill): %+v", res)
	}
	if _, err := database.FindByPath(movedPath); err != nil {
		t.Fatalf("ruta nueva no registrada: %v", err)
	}
	if same, err := database.FindByPath(movedPath); err != nil || same.ID != v.ID {
		t.Fatalf("debería seguir siendo el mismo registro")
	}
}

func TestScanDoesNotRegenerateThumbsForExisting(t *testing.T) {
	cfg, database, tmp := testEnv(t)
	dir := filepath.Join(tmp, "Videos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(dir, "clip.mp4")
	testVideo(t, video)

	res, err := Run(context.Background(), database, cfg, dir, false, nil)
	if err != nil || res.New != 1 {
		t.Fatalf("scan inicial: %+v %v", res, err)
	}
	v, err := database.FindByPath(video)
	if err != nil {
		t.Fatal(err)
	}
	thumbMtime := mtimeOf(t, v.ThumbPath)

	// Re-escaneo: la miniatura NO debe regenerarse.
	if _, err := Run(context.Background(), database, cfg, dir, false, nil); err != nil {
		t.Fatal(err)
	}
	if got := mtimeOf(t, v.ThumbPath); !got.Equal(thumbMtime) {
		t.Fatal("la miniatura de un vídeo existente se regeneró automáticamente")
	}
}

func mtimeOf(t *testing.T, path string) time.Time {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return st.ModTime()
}
