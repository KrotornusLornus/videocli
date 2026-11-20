package scan

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"videocli/internal/config"
	"videocli/internal/db"
)

// testVideo genera un mp4 diminuto con ffmpeg; salta el test si no existe.
func testVideo(t *testing.T, path string) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg no disponible")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=duration=0.2:size=64x48:rate=10",
		"-frames:v", "1", "-c:v", "mpeg4", "-y", "--", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
}

func testEnv(t *testing.T) (config.Config, *db.DB, string) {
	t.Helper()
	tmp := t.TempDir()
	cfg := config.Default()
	cfg.Paths.DataDir = filepath.Join(tmp, "data")
	cfg.Paths.CacheDir = filepath.Join(tmp, "cache")
	cfg.Paths.DownloadDir = filepath.Join(tmp, "downloads")
	cfg.Thumb.Width = 64
	cfg.Thumb.Seek = "0"
	database, err := db.Open(cfg.Paths.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return cfg, database, tmp
}

func TestScanDetectAndRelocate(t *testing.T) {
	cfg, database, tmp := testEnv(t)
	dir := filepath.Join(tmp, "Videos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(dir, "clip.mp4")
	testVideo(t, video)

	ctx := context.Background()
	res, err := Run(ctx, database, cfg, dir, false, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if res.New != 1 || len(res.Thumbs) != 1 {
		t.Fatalf("esperaba 1 nuevo y 1 miniatura: %+v", res)
	}
	if _, err := os.Stat(res.Thumbs[0]); err != nil {
		t.Fatalf("miniatura no existe: %v", err)
	}
	v, err := database.FindByPath(video)
	if err != nil {
		t.Fatalf("no registrado: %v", err)
	}

	// Segunda pasada: nada nuevo.
	res2, err := Run(ctx, database, cfg, dir, false, nil)
	if err != nil {
		t.Fatalf("segundo scan: %v", err)
	}
	if res2.New != 0 || res2.Moved != 0 || res2.Skipped != 1 {
		t.Fatalf("segunda pasada incorrecta: %+v", res2)
	}

	// Mover el fichero: debe relocalizarse por hash y no contar como nuevo.
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(sub, "clip.mp4")
	if err := os.Rename(video, moved); err != nil {
		t.Fatal(err)
	}
	res3, err := Run(ctx, database, cfg, dir, false, nil)
	if err != nil {
		t.Fatalf("tercer scan: %v", err)
	}
	if res3.New != 0 || res3.Moved != 1 {
		t.Fatalf("relocalización incorrecta: %+v", res3)
	}
	same, err := database.FindByPath(moved)
	if err != nil {
		t.Fatalf("ruta nueva no registrada: %v", err)
	}
	if same.ID != v.ID {
		t.Fatalf("debería seguir siendo el mismo registro: %d != %d", same.ID, v.ID)
	}
	if _, err := database.FindByPath(video); err != db.ErrNotFound {
		t.Fatalf("la ruta antigua debería desaparecer: %v", err)
	}
}

func TestScanSkipsHiddenAndUnknownExts(t *testing.T) {
	cfg, database, tmp := testEnv(t)
	dir := filepath.Join(tmp, "Videos")
	hidden := filepath.Join(dir, ".hidden")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	testVideo(t, filepath.Join(hidden, "nope.mp4"))
	if err := os.WriteFile(filepath.Join(dir, "leeme.txt"), []byte("hola"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), database, cfg, dir, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.New != 0 {
		t.Fatalf("debería ignorar ocultos y extensiones no válidas: %+v", res)
	}
}
