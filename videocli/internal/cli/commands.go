package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"videocli/internal/config"
	"videocli/internal/db"
	"videocli/internal/fuzzel"
	"videocli/internal/scan"
)

// cmdConfig muestra la ruta del fichero de configuración y lo crea si falta.
func cmdConfig(args []string) int {
	cfg, code := loadConfig()
	if code != 0 {
		return code
	}
	fmt.Println(cfg.File)
	return 0
}

func cmdDoctor(args []string) int {
	cfg, code := loadConfig()
	if code != 0 {
		return code
	}
	required := []string{"ffmpeg", "ffprobe", "mpv"}
	optional := []string{"yt-dlp", "fuzzel", "swayimg", "vi", "foot"}
	rc := 0
	for _, c := range required {
		if !checkCmd(c) {
			fmt.Printf("FALTA  %s (obligatorio)\n", c)
			rc = 1
		} else {
			fmt.Printf("ok     %s\n", c)
		}
	}
	for _, c := range optional {
		if !checkCmd(c) {
			fmt.Printf("falta  %s (opcional)\n", c)
		} else {
			fmt.Printf("ok     %s\n", c)
		}
	}
	if _, err := os.Stat(cfg.Paths.DBPath()); err != nil {
		fmt.Printf("falta  db: %s (se crea al primer scan)\n", cfg.Paths.DBPath())
		rc = 1
	} else {
		fmt.Printf("ok     db: %s\n", cfg.Paths.DBPath())
	}
	return rc
}

func checkCmd(c string) bool {
	_, err := exec.LookPath(c)
	return err == nil
}

func cmdScan(c *ctx) int {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	fast := fs.Bool("fast", false, "solo registrar ficheros, sin metadatos ni miniaturas")
	quiet := fs.Bool("q", false, "no mostrar progreso")
	if err := fs.Parse(c.args); err != nil {
		return 2
	}
	dir := strings.Join(fs.Args(), " ")
	if dir == "" {
		choice, err := fuzzel.ChooseDir(context.Background(), scanStartDir(c.cfg))
		if err == fuzzel.ErrCancelled {
			fmt.Fprintln(os.Stderr, "videocli: escaneo cancelado")
			return 0
		}
		if err != nil {
			return fail(1, "%v", err)
		}
		dir = choice
	}
	var log scan.Progress
	if !*quiet {
		log = func(f string, a ...any) { fmt.Fprintf(os.Stderr, "videocli: "+f+"\n", a...) }
	}
	res, err := scan.Run(context.Background(), c.dbr, c.cfg, dir, *fast, log)
	if err != nil {
		return fail(1, "%v", err)
	}
	log("nuevos=%d movidos=%d omitidos=%d errores=%d", res.New, res.Moved, res.Skipped, res.Errors)
	for _, t := range res.Thumbs {
		fmt.Println(t)
	}
	return 0
}

func scanStartDir(cfg config.Config) string {
	if len(cfg.Scan.Roots) > 0 {
		return cfg.Scan.Roots[0]
	}
	home, _ := os.UserHomeDir()
	return home
}

// jsonVideo es la representación JSON de un vídeo.
type jsonVideo struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	URL         *string `json:"url"`
	Path        string  `json:"path"`
	ThumbPath   string  `json:"thumb_path"`
	DurationMS  int64   `json:"duration_ms"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	VideoCodec  string  `json:"video_codec"`
	AudioCodec  string  `json:"audio_codec"`
	Status      string  `json:"status"`
	Source      string  `json:"source"`
	Categories  []string `json:"categories,omitempty"`
}

func toJSONVideos(videos []*db.Video, withCats bool, database *db.DB) (string, error) {
	out := make([]jsonVideo, 0, len(videos))
	for _, v := range videos {
		jv := jsonVideo{
			ID: v.ID, Title: v.Title, URL: v.URL, Path: v.Path, ThumbPath: v.ThumbPath,
			DurationMS: v.DurationMS, Width: v.Width, Height: v.Height,
			VideoCodec: v.VideoCodec, AudioCodec: v.AudioCodec,
			Status: v.Status, Source: v.Source,
		}
		if withCats {
			cats, err := database.ListVideoCategories(v.ID)
			if err != nil {
				return "", err
			}
			jv.Categories = cats
		}
		out = append(out, jv)
	}
	data, err := json.MarshalIndent(out, "", "  ")
	return string(data), err
}

func cmdList(c *ctx) int {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	category := fs.String("category", "", "filtrar por categoría (hereda subcategorías; \"sin-categoria\" = sin categorías)")
	dir := fs.String("dir", "", "solo vídeos bajo este directorio")
	asJSON := fs.Bool("json", false, "salida JSON")
	if err := fs.Parse(c.args); err != nil {
		return 2
	}

	if *dir != "" {
		*dir = expandTilde(*dir)
		if abs, aerr := filepath.Abs(*dir); aerr == nil {
			*dir = abs
		}
	}
	videos, err := c.dbr.ListVideos(*category, *dir)
	if err != nil {
		return fail(1, "%v", err)
	}
	if *asJSON {
		data, err := toJSONVideos(videos, true, c.dbr)
		if err != nil {
			return fail(1, "%v", err)
		}
		fmt.Println(data)
		return 0
	}
	for _, v := range videos {
		fmt.Println(v.ThumbPath)
	}
	return 0
}

func cmdCategories(c *ctx) int {
	cats, err := c.dbr.ListCategories()
	if err != nil {
		return fail(1, "%v", err)
	}
	for _, ct := range cats {
		fmt.Println(ct.Path)
	}
	return 0
}

// cmdPickDir abre el selector de carpetas (fuzzel) y escribe la elegida
// en stdout. Empieza en ~/Videos, o en la carpeta indicada como argumento.
func cmdPickDir(c *ctx) int {
	start := ""
	if len(c.args) > 0 && statIsDir(expandTilde(c.args[0])) {
		start = expandTilde(c.args[0])
	} else {
		if home, err := os.UserHomeDir(); err == nil {
			videos := filepath.Join(home, "Videos")
			if statIsDir(videos) {
				start = videos
			}
		}
	}
	if start == "" {
		if home, err := os.UserHomeDir(); err == nil {
			start = home
		}
	}
	choice, err := fuzzel.ChooseDir(context.Background(), start)
	if err != nil {
		return 0
	}
	fmt.Println(choice)
	return 0
}

func statIsDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// expandTilde resuelve ~ y ~/ con el HOME del usuario.
func expandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// cmdUrl imprime la URL de origen del vídeo (entrada importada).
// Si no tiene URL, no imprime nada y sale 1.
func cmdUrl(c *ctx) int {
	if len(c.args) != 1 {
		return fail(2, "uso: videocli url <ruta|id>")
	}
	v, err := resolveVideo(c, c.args[0])
	if err != nil {
		return fail(1, "vídeo no encontrado: %s", c.args[0])
	}
	if v.URL == nil || *v.URL == "" {
		return fail(1, "el vídeo no tiene URL de origen")
	}
	fmt.Println(*v.URL)
	return 0
}

// cmdLocate imprime la ruta local del vídeo si tiene fichero.
// Si no, no imprime nada y sale 1.
func cmdLocate(c *ctx) int {
	if len(c.args) != 1 {
		return fail(2, "uso: videocli locate <ruta|id>")
	}
	v, err := resolveVideo(c, c.args[0])
	if err != nil {
		return fail(1, "vídeo no encontrado: %s", c.args[0])
	}
	if v.Path == "" {
		return fail(1, "el vídeo no tiene fichero local")
	}
	fmt.Println(v.Path)
	return 0
}

// cmdInfo imprime un resumen en una línea para la capa de estado de swayimg.
func cmdInfo(c *ctx) int {
	if len(c.args) != 1 {
		return fail(2, "uso: videocli info <ruta|id>")
	}
	v, err := resolveVideo(c, c.args[0])
	if err != nil {
		return fail(1, "vídeo no encontrado: %s", c.args[0])
	}
	fields := []string{}
	if v.Title != "" {
		fields = append(fields, v.Title)
	}
	if v.DurationMS > 0 {
		d := time.Duration(v.DurationMS) * time.Millisecond
		fields = append(fields, d.Truncate(time.Second).String())
	}
	if v.Width > 0 {
		fields = append(fields, fmt.Sprintf("%dx%d", v.Width, v.Height))
	}
	if v.VideoCodec != "" {
		fields = append(fields, v.VideoCodec)
	}
	if v.AudioCodec != "" {
		fields = append(fields, v.AudioCodec)
	}
	cats, err := c.dbr.ListVideoCategories(v.ID)
	if err != nil {
		return fail(1, "%v", err)
	}
	if len(cats) > 0 {
		fields = append(fields, "categorías: "+strings.Join(cats, ", "))
	}
	if len(fields) == 0 {
		fields = append(fields, filepath.Base(v.Path))
	}
	fmt.Println(strings.Join(fields, "  ·  "))
	return 0
}
