// Package download implementa la importación de metadatos y la descarga
// de vídeos con yt-dlp. Todo lo ejecuta el CLI; no hay procesos externos.
package download

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"videocli/internal/config"
	"videocli/internal/db"
	"videocli/internal/hash"
	"videocli/internal/meta"
	"videocli/internal/thumb"
)

// metadata es el subconjunto del JSON de yt-dlp que usamos.
type metadata struct {
	Title      string  `json:"title"`
	Duration   float64 `json:"duration"`
	WebpageURL string  `json:"webpage_url"`
}

// parseOutput separa las líneas de salida de yt-dlp: una con el JSON de
// metadatos y otra con la ruta final (after_move:filepath).
func parseOutput(lines []string) (*metadata, string, error) {
	var meta *metadata
	var filePath string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "{") {
			var m metadata
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				return nil, "", fmt.Errorf("JSON de yt-dlp: %w", err)
			}
			meta = &m
		} else if strings.HasPrefix(line, "/") {
			// Solo una ruta absoluta cuenta como salida de after_move:filepath;
			// el resto es ruido de yt-dlp.
			filePath = line
		}
	}
	return meta, filePath, nil
}

// acquireLock limita la concurrencia con un flock sobre el bloqueo de la caché.
func acquireLock(cfg config.Config) (*os.File, error) {
	if err := os.MkdirAll(cfg.Paths.CacheDir, 0o755); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(cfg.Paths.CacheDir, "download.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("esperando el bloqueo de descargas: %w", err)
	}
	return f, nil
}

// runYtDlp ejecuta yt-dlp y devuelve las líneas de stdout; stderr al log.
func runYtDlp(ctx context.Context, cfg config.Config, args []string) ([]string, error) {
	cmd := exec.CommandContext(ctx, cfg.Download.Command, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	if err := os.MkdirAll(cfg.Paths.LogsDir(), 0o755); err != nil {
		return nil, err
	}
	logPath := filepath.Join(cfg.Paths.LogsDir(),
		fmt.Sprintf("yt-dlp-%d.log", time.Now().UnixNano()))
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var lines []string
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := cmd.Wait(); err != nil {
		os.WriteFile(logPath, []byte(errBuf.String()), 0o644)
		return lines, fmt.Errorf("%s: %w (log: %s)", cfg.Download.Command, err, logPath)
	}
	return lines, nil
}

// notify lanza el comando de notificación si está activado.
func notify(cfg config.Config, msg string) {
	if !cfg.Notify.Enabled {
		return
	}
	cmd := exec.Command(cfg.Notify.Command, "Pi", msg)
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Start()
}

// completeLocal completa metadatos y hash a partir del fichero local.
// La miniatura SOLO se genera si el registro no tenía ninguna
// (las miniaturas existentes nunca se regeneran automáticamente).
func completeLocal(ctx context.Context, database *db.DB, cfg config.Config, v *db.Video, path string) error {
	info, err := meta.Probe(ctx, path)
	if err != nil {
		return fmt.Errorf("metadatos: %w", err)
	}
	if err := database.UpdateVideoMeta(v.ID, info.DurationMS, info.Width, info.Height,
		info.VideoCodec, info.AudioCodec); err != nil {
		return err
	}
	// Hash: imprescindible para que la re-localización funcione si el
	// fichero se mueve más tarde.
	sum, size, err := hash.Read(path, 64*1024)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}
	if err := database.UpdateVideoHash(v.ID, sum, size); err != nil {
		return err
	}
	if v.ThumbPath != "" {
		// Ya tenía miniatura (online u otra): no se toca.
		return nil
	}
	thumbPath := filepath.Join(cfg.Paths.ThumbsDir(), fmt.Sprintf("%d.jpg", v.ID))
	if err := thumb.Generate(ctx, "local", path, "", thumbPath, thumb.Options{
		Width: cfg.Thumb.Width, Quality: cfg.Thumb.Quality, Seek: cfg.Thumb.Seek,
	}); err != nil {
		return fmt.Errorf("miniatura: %w", err)
	}
	return database.UpdateVideoThumb(v.ID, thumbPath)
}

// Import obtiene los metadatos de una URL sin descargar el vídeo:
// título y duración desde el JSON de yt-dlp, miniatura online.
func Import(ctx context.Context, database *db.DB, cfg config.Config, id int64) error {
	v, err := database.GetVideo(id)
	if err != nil {
		return err
	}
	if v.URL == nil || *v.URL == "" {
		return fmt.Errorf("el vídeo %d no tiene URL", id)
	}
	lock, err := acquireLock(cfg)
	if err != nil {
		return err
	}
	defer lock.Close()

	args := []string{"--skip-download", "--no-simulate", "--dump-json", "--", *v.URL}
	lines, err := runYtDlp(ctx, cfg, args)
	if err != nil {
		database.UpdateVideoStatus(id, "error", err.Error())
		return err
	}
	metaData, _, err := parseOutput(lines)
	if err != nil || metaData == nil {
		msg := fmt.Sprint("sin JSON de metadatos")
		if err != nil {
			msg = err.Error()
		}
		database.UpdateVideoStatus(id, "error", msg)
		return errors.New(msg)
	}
	if metaData.Title != "" {
		if err := database.UpdateVideoTitle(id, metaData.Title); err != nil {
			return err
		}
		v.Title = metaData.Title
	}
	duration := int64(metaData.Duration * 1000)
	if err := database.UpdateVideoMeta(id, duration, 0, 0, "", ""); err != nil {
		return err
	}
	// Miniatura online (yt-dlp) para que se vea en la galería.
	thumbPath := filepath.Join(cfg.Paths.ThumbsDir(), fmt.Sprintf("%d.jpg", id))
	opt := thumb.Options{Width: cfg.Thumb.Width, Quality: cfg.Thumb.Quality, Seek: cfg.Thumb.Seek}
	if err := thumb.Generate(ctx, "online", "", *v.URL, thumbPath, opt); err != nil {
		// Fallback: placeholder; se puede regenerar después (tecla r).
		if phErr := thumb.GeneratePlaceholder(ctx, thumbPath, opt); phErr != nil {
			database.UpdateVideoStatus(id, "error", "miniatura online: "+err.Error())
			return err
		}
		database.UpdateVideoStatus(id, "url", "miniatura pendiente: regenera con r")
	}
	if err := database.UpdateVideoThumb(id, thumbPath); err != nil {
		return err
	}
	if status, _ := database.GetVideo(id); status != nil && status.Status != "url" {
		if err := database.UpdateVideoStatus(id, "url", ""); err != nil {
			return err
		}
	}
	notify(cfg, fmt.Sprintf("Importado: %s", v.Title))
	return nil
}

// Fetch descarga el vídeo de una entrada importada (solo metadatos hasta
// ahora) con calidad máxima maxH (0 = mejor disponible) en destDir
// (vacío = download_dir de la configuración). Actualiza path, origen y
// miniatura local.
func Fetch(ctx context.Context, database *db.DB, cfg config.Config, id int64, maxH int64, destDir string) error {
	v, err := database.GetVideo(id)
	if err != nil {
		return err
	}
	if v.URL == nil || *v.URL == "" {
		return fmt.Errorf("el vídeo %d no tiene URL de origen", id)
	}
	if v.Path != "" {
		return fmt.Errorf("el vídeo %d ya está descargado (%s)", id, v.Path)
	}
	if destDir == "" {
		destDir = cfg.Paths.DownloadDir
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	lock, err := acquireLock(cfg)
	if err != nil {
		return err
	}
	defer lock.Close()

	if err := database.UpdateVideoStatus(id, "downloading", ""); err != nil {
		return err
	}
	// Sustituye {dir} y añade el JSON de metadatos a los argumentos configurados.
	args := make([]string, 0, len(cfg.Download.Args)+4)
	for _, a := range cfg.Download.Args {
		args = append(args, strings.ReplaceAll(a, "{dir}", destDir))
	}
	if maxH > 0 {
		// Límite de calidad: mejor vídeo <= maxH + mejor audio.
		args = append(args, "-f",
			fmt.Sprintf("bv*[height<=%d]+ba/b[height<=%d]/b", maxH, maxH))
	}
	args = append(args, "--dump-json", "--", *v.URL)
	lines, err := runYtDlp(ctx, cfg, args)
	if err != nil {
		database.UpdateVideoStatus(id, "error", err.Error())
		return err
	}
	metaData, filePath, err := parseOutput(lines)
	if err != nil {
		database.UpdateVideoStatus(id, "error", err.Error())
		return err
	}
	if filePath == "" {
		msg := "yt-dlp no devolvió la ruta final"
		database.UpdateVideoStatus(id, "error", msg)
		return errors.New(msg)
	}
	if _, statErr := os.Stat(filePath); statErr != nil {
		msg := fmt.Sprintf("la ruta devuelta no existe: %s", filePath)
		database.UpdateVideoStatus(id, "error", msg)
		return errors.New(msg)
	}
	if err := database.UpdateVideoPath(id, filePath); err != nil {
		return err
	}
	if err := database.UpdateVideoSource(id, "download"); err != nil {
		return err
	}
	if metaData != nil && metaData.Title != "" && v.Title == "" {
		_ = database.UpdateVideoTitle(id, metaData.Title)
	}
	if err := completeLocal(ctx, database, cfg, v, filePath); err != nil {
		// El vídeo está descargado; solo falló metadato/miniatura.
		database.UpdateVideoStatus(id, "error", err.Error())
		return err
	}
	if err := database.UpdateVideoStatus(id, "ok", ""); err != nil {
		return err
	}
	notify(cfg, fmt.Sprintf("Descargado: %s", v.Title))
	return nil
}
