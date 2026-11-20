// Package thumb genera miniaturas con ffmpeg (local) o yt-dlp (online).
package thumb

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Options parametriza la generación de miniaturas.
type Options struct {
	// Width es el ancho en píxeles.
	Width int
	// Quality es el -q:v de ffmpeg.
	Quality int
	// Seek es "10%" (porcentaje de duración) o segundos ("1.5").
	Seek string
}

// generateLocal crea un .jpg a partir de un fotograma de un vídeo local.
func generateLocal(ctx context.Context, videoPath, outPath string, opt Options) error {
	seek := seekSeconds(videoPath, opt)
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-ss", fmtFloat(seek),
		"-i", videoPath,
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:-2", opt.Width),
		"-q:v", fmt.Sprintf("%d", opt.Quality),
		"-y", outPath,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg %s: %w: %s", videoPath, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// generateOnline descarga la miniatura de una URL con yt-dlp.
func generateOnline(ctx context.Context, url, outPath string, opt Options) error {
	tmp, err := os.MkdirTemp("", "videocli-thumb-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	pattern := filepath.Join(tmp, "cover.%(ext)s")
	args := []string{
		"--skip-download", "--no-simulate",
		"--write-thumbnail", "--convert-thumbnails", "jpg",
		"-o", pattern,
		"--", url,
	}
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("yt-dlp %s: %w: %s", url, err, strings.TrimSpace(string(out)))
	}
	matches, err := filepath.Glob(filepath.Join(tmp, "cover.*"))
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("yt-dlp no produjo miniatura para %s", url)
	}
	src := matches[0]
	// Reescalar al ancho deseado con ffmpeg.
	tmp2, err := os.MkdirTemp("", "videocli-thumb2-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp2)
	scaled := filepath.Join(tmp2, "scaled.jpg")
	cmd2 := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", src,
		"-vf", fmt.Sprintf("scale=%d:-2", opt.Width),
		"-q:v", fmt.Sprintf("%d", opt.Quality),
		"-y", scaled)
	if out, err := cmd2.CombinedOutput(); err != nil {
		return fmt.Errorf("escalar miniatura: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	// Copia (no rename: /tmp puede estar en otro sistema de ficheros).
	srcF, err := os.Open(scaled)
	if err != nil {
		return err
	}
	defer srcF.Close()
	dstF, err := os.Create(outPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dstF, srcF); err != nil {
		dstF.Close()
		return err
	}
	return dstF.Close()
}

// seekSeconds interpreta opt.Seek ("10%", "1.5") con la duración del vídeo.
func seekSeconds(videoPath string, opt Options) float64 {
	seek := strings.TrimSpace(opt.Seek)
	if strings.HasSuffix(seek, "%") {
		pct, err := strconvParseFloat(strings.TrimSuffix(seek, "%"))
		if err != nil || pct < 0 {
			pct = 10
		}
		dur := durationSeconds(videoPath)
		if dur <= 0 {
			return 1
		}
		s := dur * pct / 100
		if s < 0.5 {
			s = 0.5
		}
		if s > dur-0.5 {
			s = math.Max(0, dur-0.5)
		}
		return s
	}
	if s, err := strconvParseFloat(seek); err == nil && s >= 0 {
		return s
	}
	return 1
}

func strconvParseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%g", &f)
	return f, err
}

// durationSeconds consulta la duración con ffprobe; 0 si falla.
func durationSeconds(videoPath string) float64 {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", "--", videoPath)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(out))
	if s == "" || s == "N/A" {
		return 0
	}
	f, err := strconvParseFloat(s)
	if err != nil {
		return 0
	}
	return f
}

func fmtFloat(f float64) string {
	return fmt.Sprintf("%.3f", f)
}

// GeneratePlaceholder crea una miniatura sustituta (carta de ajuste) para
// vídeos cuya miniatura real no se pudo generar. Se sobrescribe al regenerar.
func GeneratePlaceholder(ctx context.Context, outPath string, opt Options) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc=duration=1:size=%dx270:rate=1", opt.Width),
		"-frames:v", "1",
		"-q:v", fmt.Sprintf("%d", opt.Quality),
		"-y", outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("placeholder: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Generate crea (o regenera) la miniatura en outPath.
// method es "local" (requiere videoPath) u "online" (requiere url).
func Generate(ctx context.Context, method, videoPath, url, outPath string, opt Options) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	switch method {
	case "local":
		if videoPath == "" {
			return fmt.Errorf("método local: el vídeo no tiene fichero local")
		}
		return generateLocal(ctx, videoPath, outPath, opt)
	case "online":
		if url == "" {
			return fmt.Errorf("método online: el vídeo no tiene URL de origen")
		}
		return generateOnline(ctx, url, outPath, opt)
	default:
		return fmt.Errorf("método desconocido: %s", method)
	}
}
