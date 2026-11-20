// Package config carga y normaliza la configuración de videocli.
//
// La configuración vive en $XDG_CONFIG_HOME/videocli/config.toml y sigue
// la especificación XDG para datos ($XDG_DATA_HOME) y caché ($XDG_CACHE_HOME).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config es la configuración completa de videocli.
type Config struct {
	Paths    Paths    `toml:"paths"`
	Scan     Scan     `toml:"scan"`
	Thumb    Thumb    `toml:"thumb"`
	Player   Player   `toml:"player"`
	Download Download `toml:"download"`
	Notify   Notify   `toml:"notify"`

	// File es la ruta del fichero de configuración cargado.
	File string `toml:"-"`
}

// Paths define los directorios usados por videocli.
type Paths struct {
	// DataDir contiene library.db. Por defecto $XDG_DATA_HOME/videocli.
	DataDir string `toml:"data_dir"`
	// CacheDir contiene miniaturas y logs regenerables. Por defecto
	// $XDG_CACHE_HOME/videocli.
	CacheDir string `toml:"cache_dir"`
	// DownloadDir es donde yt-dlp guarda las descargas.
	DownloadDir string `toml:"download_dir"`
}

// DBPath devuelve la ruta de la base de datos.
func (p Paths) DBPath() string { return filepath.Join(p.DataDir, "library.db") }

// ThumbsDir devuelve el directorio de miniaturas.
func (p Paths) ThumbsDir() string { return filepath.Join(p.CacheDir, "thumbs") }

// LogsDir devuelve el directorio de logs de descargas.
func (p Paths) LogsDir() string { return filepath.Join(p.CacheDir, "logs") }

// Scan define qué se escanea y desde dónde.
type Scan struct {
	// Roots son las carpetas mostradas por el selector fuzzel.
	Roots []string `toml:"roots"`
	// Extensions son las extensiones consideradas vídeo.
	Extensions []string `toml:"extensions"`
}

// Thumb define cómo se generan las miniaturas.
type Thumb struct {
	// Width es el ancho en píxeles de la miniatura.
	Width int `toml:"width"`
	// Quality es el -q:v de ffmpeg (2-5 es razonable).
	Quality int `toml:"quality"`
	// Seek es el instante del fotograma: "10%" o "1.5" (segundos).
	Seek string `toml:"seek"`
}

// Player define el reproductor.
type Player struct {
	Command   string   `toml:"command"`
	Args      []string `toml:"args"`
	MusicArgs []string `toml:"music_args"`
}

// Download define el descargador.
type Download struct {
	Command       string   `toml:"command"`
	Args          []string `toml:"args"`
	MaxConcurrent int      `toml:"max_concurrent"`
	// Qualities son las alturas máximas (en píxeles) ofrecidas por el
	// selector de calidad al descargar. Sin límite = "mejor disponible".
	Qualities []int `toml:"qualities"`
}

// Notify define las notificaciones de escritorio.
type Notify struct {
	Enabled bool   `toml:"enabled"`
	Command string `toml:"command"`
}

// Default devuelve la configuración por defecto a partir del entorno XDG.
func Default() Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	cfg := Config{
		Paths: Paths{
			DataDir:     filepath.Join(xdgDataHome(home), "videocli"),
			CacheDir:    filepath.Join(xdgCacheHome(home), "videocli"),
			DownloadDir: filepath.Join(home, "Videos", "videocli"),
		},
		Scan: Scan{
			Roots: []string{
				filepath.Join(home, "Videos"),
				filepath.Join(home, "Descargas"),
				filepath.Join(home, "Downloads"),
			},
			Extensions: defaultExtensions(),
		},
		Thumb:  Thumb{Width: 480, Quality: 3, Seek: "10%"},
		Player: Player{Command: "mpv", MusicArgs: []string{"--no-video"}},
		Download: Download{
			Command:       "yt-dlp",
			Args:          []string{"-P", "{dir}", "-o", "%(title)s [%(id)s].%(ext)s", "--print", "after_move:filepath", "--no-simulate"},
			MaxConcurrent: 1,
			Qualities:     []int{2160, 1440, 1080, 720, 480, 360},
		},
		Notify: Notify{Enabled: false, Command: "fyi"},
	}
	cfg.File = ConfigFile()
	return cfg
}

func defaultExtensions() []string {
	return []string{
		".mp4", ".mkv", ".webm", ".avi", ".mov", ".m4v", ".flv",
		".wmv", ".mpg", ".mpeg", ".ts", ".m2ts", ".3gp", ".ogv", ".vob",
	}
}

func xdgDataHome(home string) string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	return filepath.Join(home, ".local", "share")
}

func xdgCacheHome(home string) string {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return v
	}
	return filepath.Join(home, ".cache")
}

// ConfigDir devuelve $XDG_CONFIG_HOME/videocli.
func ConfigDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ".config/videocli"
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "videocli")
}

// ConfigFile devuelve la ruta del fichero de configuración.
func ConfigFile() string { return filepath.Join(ConfigDir(), "config.toml") }

// Load lee la configuración del disco y la normaliza.
// Si no existe el fichero, crea uno con los valores por defecto.
func Load() (Config, error) {
	cfg := Default()
	path := cfg.File
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if werr := writeDefaultConfig(path); werr == nil {
			data, err = os.ReadFile(path)
		} else {
			// Sin config no se puede escribir: seguimos con los defaults.
			cfg.normalize(nil)
			return cfg, nil
		}
	}
	if err != nil {
		return cfg, err
	}
	meta, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return cfg, fmt.Errorf("config %s: %w", path, err)
	}
	cfg.normalize(&meta)
	return cfg, nil
}

// writeDefaultConfig crea el config.toml por defecto en la primera ejecución.
func writeDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(DefaultConfigTOML), 0o644)
}

// normalize expande rutas, aplica valores por defecto y limpia extensiones.
func (c *Config) normalize(meta *toml.MetaData) {	c.Paths.DataDir = expandDefault(c.Paths.DataDir, func() string {
		return Default().Paths.DataDir
	})
	c.Paths.CacheDir = expandDefault(c.Paths.CacheDir, func() string {
		return Default().Paths.CacheDir
	})
	c.Paths.DownloadDir = expandDefault(c.Paths.DownloadDir, func() string {
		return Default().Paths.DownloadDir
	})
	for i, r := range c.Scan.Roots {
		c.Scan.Roots[i] = expand(r)
	}
	for i, e := range c.Scan.Extensions {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" && !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		c.Scan.Extensions[i] = e
	}
	if len(c.Scan.Extensions) == 0 {
		c.Scan.Extensions = defaultExtensions()
	}
	if c.Thumb.Width <= 0 {
		c.Thumb.Width = 480
	}
	if c.Thumb.Quality <= 0 {
		c.Thumb.Quality = 3
	}
	if c.Thumb.Seek == "" {
		c.Thumb.Seek = "10%"
	}
	if c.Player.Command == "" {
		c.Player.Command = "mpv"
	}
	if c.Download.Command == "" {
		c.Download.Command = "yt-dlp"
	}
	if c.Download.MaxConcurrent <= 0 {
		c.Download.MaxConcurrent = 1
	}
	if len(c.Download.Qualities) == 0 {
		c.Download.Qualities = []int{2160, 1440, 1080, 720, 480, 360}
	}
	if c.Notify.Command == "" {
		c.Notify.Command = "fyi"
	}

	// Aviso de claves desconocidas para detectar erratas.
	if meta != nil {
		var unknown []string
		for _, k := range meta.Undecoded() {
			unknown = append(unknown, k.String())
		}
		if len(unknown) > 0 {
			fmt.Fprintf(os.Stderr, "videocli: claves desconocidas en %s: %s\n",
				c.File, strings.Join(unknown, ", "))
		}
	}
}

func expandDefault(v string, def func() string) string {
	if v == "" {
		return def()
	}
	return expand(v)
}

// expand resuelve ~ y variables de entorno en una ruta.
func expand(p string) string {
	p = os.ExpandEnv(p)
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~"+string(os.PathSeparator)) {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// DefaultConfigTOML es el contenido del config.toml que escribe `init`.
const DefaultConfigTOML = `# Configuración de videocli.
# Los datos (library.db) van a $XDG_DATA_HOME/videocli y la caché
# (miniaturas, logs) a $XDG_CACHE_HOME/videocli, salvo que se sobrescriban aquí.

[paths]
# Carpeta donde yt-dlp guarda las descargas:
download_dir = "~/Videos/videocli"

[scan]
# Carpetas que muestra el selector fuzzel al escanear (tecla s):
roots = ["~/Videos", "~/Descargas", "~/Downloads"]
# Extensiones que se consideran vídeo:
extensions = [".mp4", ".mkv", ".webm", ".avi", ".mov", ".m4v", ".flv",
              ".wmv", ".mpg", ".mpeg", ".ts", ".m2ts", ".3gp", ".ogv", ".vob"]

[thumb]
width = 480
quality = 3
# Instantáneo del fotograma: porcentaje de duración ("10%") o segundos ("1.5"):
seek = "10%"

[player]
command = "mpv"
music_args = ["--no-video"]

[download]
command = "yt-dlp"
# {dir} se sustituye por el directorio de descargas:
args = ["-P", "{dir}", "-o", "%(title)s [%(id)s].%(ext)s",
        "--print", "after_move:filepath", "--no-simulate"]
max_concurrent = 1
# Alturas máximas ofrecidas al elegir calidad de descarga:
qualities = [2160, 1440, 1080, 720, 480, 360]

[notify]
enabled = false
command = "fyi"
`
