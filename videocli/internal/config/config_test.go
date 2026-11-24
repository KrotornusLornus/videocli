package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWithoutFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	t.Setenv("HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Paths.DataDir != filepath.Join(tmp, ".local", "share", "videocli") {
		t.Fatalf("DataDir: %s", cfg.Paths.DataDir)
	}
	if cfg.Paths.CacheDir != filepath.Join(tmp, "cache", "videocli") {
		t.Fatalf("CacheDir: %s", cfg.Paths.CacheDir)
	}
	if cfg.Player.Command != "mpv" || cfg.Thumb.Width != 480 {
		t.Fatalf("defaults: %+v", cfg)
	}
}

func TestLoadWithOverrides(t *testing.T) {
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "config", "videocli")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `
[thumb]
width = 320
seek = "2.5"

[download]
max_concurrent = 2
qualities = [1080, 720]
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	// Paths y Scan ya no son configurables: siempre los valores XDG.
	if cfg.Paths.DataDir != filepath.Join(tmp, ".local", "share", "videocli") {
		t.Fatalf("DataDir debería seguir XDG: %s", cfg.Paths.DataDir)
	}
	if cfg.Download.MaxConcurrent != 2 {
		t.Fatalf("max_concurrent override: %d", cfg.Download.MaxConcurrent)
	}
	if len(cfg.Download.Qualities) != 2 || cfg.Download.Qualities[0] != 1080 {
		t.Fatalf("qualities override: %v", cfg.Download.Qualities)
	}
	if cfg.Thumb.Width != 320 || cfg.Thumb.Seek != "2.5" {
		t.Fatalf("thumb override: %+v", cfg.Thumb)
	}
	// Defaults intactos para claves no presentes.
	if cfg.Player.Command != "mpv" {
		t.Fatalf("player: %+v", cfg.Player)
	}
}

func TestDefaultConfigTOMLParses(t *testing.T) {
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "videocli")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(DefaultConfigTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("el config por defecto debería parsear: %v", err)
	}
	if cfg.Paths.DownloadDir == "" || cfg.Thumb.Seek == "" {
		t.Fatalf("config por defecto incompleta: %+v", cfg)
	}
}
