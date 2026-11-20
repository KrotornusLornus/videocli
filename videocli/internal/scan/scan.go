// Package scan recorre carpetas y registra vídeos en la base de datos.
package scan

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"videocli/internal/config"
	"videocli/internal/db"
	"videocli/internal/hash"
	"videocli/internal/meta"
	"videocli/internal/thumb"
)

// Result resume una pasada de escaneo.
type Result struct {
	// Thumbs son las rutas de miniaturas de vídeos nuevos (salida de stdout).
	Thumbs []string
	// New son los vídeos registrados por primera vez.
	New int
	// Moved son los vídeos relocalizados por hash.
	Moved int
	// Skipped son los ficheros ya conocidos.
	Skipped int
	// Errors son los fallos por fichero.
	Errors int
}

// Progress recibe avisos para stderr.
type Progress func(format string, args ...any)

// Run recorre dir de forma recursiva y devuelve las miniaturas nuevas.
// Con fast=true omite metadatos y miniatura.
func Run(ctx context.Context, database *db.DB, cfg config.Config, dir string, fast bool, log Progress) (*Result, error) {
	if log == nil {
		log = func(string, ...any) {}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("%s no es una carpeta", abs)
	}
	exts := map[string]bool{}
	for _, e := range cfg.Scan.Extensions {
		exts[strings.ToLower(e)] = true
	}
	res := &Result{}

	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		// Ignorar ocultos y descendir solo en carpetas.
		if d.IsDir() {
			if path != abs && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || !exts[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		if err := processFile(ctx, database, cfg, path, fast, res, log); err != nil {
			res.Errors++
			log("error: %v", err)
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	sort.Strings(res.Thumbs)
	return res, nil
}

func processFile(ctx context.Context, database *db.DB, cfg config.Config, path string, fast bool, res *Result, log Progress) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if existing, ferr := database.FindByPath(abs); ferr == nil {
		res.Skipped++
		log("ya registrado: %s", abs)
		// Regenerar si falta la miniatura, o si el fichero de miniatura
		// desapareció de la caché.
		if existing != nil && !fast && (existing.ThumbPath == "" ||
			func() bool { _, statErr := os.Stat(existing.ThumbPath); return os.IsNotExist(statErr) }()) {
			if err := ensureThumb(ctx, database, cfg, existing); err != nil {
				return err
			}
		}
		return nil
	} else if ferr != db.ErrNotFound {
		return ferr
	}

	sum, size, err := hash.Read(abs, 64*1024)
	if err != nil {
		return err
	}
	byHash, err := database.FindByHash(sum, size)
	switch {
	case err == nil && byHash != nil:
		if _, statErr := os.Stat(byHash.Path); os.IsNotExist(statErr) {
			// Fichero movido: relocalizar sin tratarlo como nuevo.
			log("movido: %s -> %s", byHash.Path, abs)
			if err := database.UpdateVideoPath(byHash.ID, abs); err != nil {
				return err
			}
			res.Moved++
			return nil
		}
		// Copia distinta con el mismo contenido inicial: registro nuevo.
	case err != nil && err != db.ErrNotFound:
		return err
	}

	title := strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	v := &db.Video{Title: title, Path: abs, ContentHash: sum, HashSize: size}
	id, err := database.InsertVideo(v)
	if err != nil {
		return err
	}
	res.New++
	if fast {
		log("registrado (--fast): %s", abs)
		return nil
	}
	if err := ensureThumb(ctx, database, cfg, v); err != nil {
		return err
	}
	if v.ThumbPath != "" {
		res.Thumbs = append(res.Thumbs, v.ThumbPath)
	}
	_ = id
	return nil
}

// ensureThumb completa metadatos y miniatura de un vídeo recién insertado.
// Si la miniatura falla, deja un placeholder para que el vídeo aparezca
// igualmente en la galería (se puede regenerar con la tecla r).
func ensureThumb(ctx context.Context, database *db.DB, cfg config.Config, v *db.Video) error {
	thumbPath := filepath.Join(cfg.Paths.ThumbsDir(), fmt.Sprintf("%d.jpg", v.ID))
	opt := thumb.Options{Width: cfg.Thumb.Width, Quality: cfg.Thumb.Quality, Seek: cfg.Thumb.Seek}
	if err := thumb.Generate(ctx, "local", v.Path, "", thumbPath, opt); err != nil {
		// Fallback: placeholder para que se vea en la galería y se pueda
		// regenerar después. El error se guarda en el campo error.
		if phErr := thumb.GeneratePlaceholder(ctx, thumbPath, opt); phErr != nil {
			return fmt.Errorf("miniatura %s: %w: %v", v.Path, err, phErr)
		}
		if err := database.UpdateVideoStatus(v.ID, v.Status, "miniatura pendiente: regenera con r"); err != nil {
			return err
		}
	}
	info, err := meta.Probe(ctx, v.Path)
	if err != nil {
		// Metadatos no críticos: el vídeo es reproducible sin ellos.
		return database.UpdateVideoThumb(v.ID, thumbPath)
	}
	if info.Title != "" {
		v.Title = info.Title
		_ = database.UpdateVideoTitle(v.ID, info.Title)
	}
	if err := database.UpdateVideoMeta(v.ID, info.DurationMS, info.Width, info.Height,
		info.VideoCodec, info.AudioCodec); err != nil {
		return err
	}
	if err := database.UpdateVideoThumb(v.ID, thumbPath); err != nil {
		return err
	}
	v.ThumbPath = thumbPath
	return nil
}
