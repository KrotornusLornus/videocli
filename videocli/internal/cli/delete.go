package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"videocli/internal/thumb"
)

// cmdRemove elimina registros de la base de datos (metadatos), borrando
// también la miniatura en caché. El fichero local no se toca.
func cmdRemove(c *ctx) int {
	if len(c.args) == 0 {
		return fail(2, "uso: videocli remove RUTA|ID...")
	}
	removed := 0
	for _, ref := range c.args {
		v, err := resolveVideo(c, ref)
		if err != nil {
			fmt.Fprintf(os.Stderr, "videocli: no encontrado: %s\n", ref)
			continue
		}
		if v.ThumbPath != "" {
			_ = os.Remove(v.ThumbPath)
		}
		if err := c.dbr.DeleteVideo(v.ID); err != nil {
			fmt.Fprintf(os.Stderr, "videocli: %v\n", err)
			continue
		}
		removed++
	}
	fmt.Fprintf(os.Stderr, "videocli: %d registro(s) eliminados de la base de datos\n", removed)
	return 0
}

// cmdDeleteFile borra el fichero de vídeo local pero conserva los
// metadatos. Si la entrada es solo-URL no hay nada que borrar.
func cmdDeleteFile(c *ctx) int {
	if len(c.args) == 0 {
		return fail(2, "uso: videocli delete-file RUTA|ID...")
	}
	deleted := 0
	for _, ref := range c.args {
		v, err := resolveVideo(c, ref)
		if err != nil {
			fmt.Fprintf(os.Stderr, "videocli: no encontrado: %s\n", ref)
			continue
		}
		if v.Path == "" {
			fmt.Fprintf(os.Stderr, "videocli: %s no tiene fichero local\n", ref)
			continue
		}
		if err := os.Remove(v.Path); err != nil {
			fmt.Fprintf(os.Stderr, "videocli: %v\n", err)
			continue
		}
		if err := c.dbr.ClearVideoPath(v.ID); err != nil {
			fmt.Fprintf(os.Stderr, "videocli: %v\n", err)
			continue
		}
		// Placeholder en vez de miniatura: el vídeo sigue en la galería
		// (sin fichero local, con sus metadatos) y se puede regenerar.
		thumbPath := filepath.Join(c.cfg.Paths.ThumbsDir(), fmt.Sprintf("%d.jpg", v.ID))
		if _, statErr := os.Stat(thumbPath); os.IsNotExist(statErr) {
			_ = thumb.GeneratePlaceholder(context.Background(), thumbPath,
				thumb.Options{Width: c.cfg.Thumb.Width, Quality: c.cfg.Thumb.Quality})
		}
		_ = c.dbr.UpdateVideoThumb(v.ID, thumbPath)
		_ = c.dbr.UpdateVideoStatus(v.ID, v.Status, "fichero local eliminado")
		deleted++
	}
	fmt.Fprintf(os.Stderr, "videocli: %d fichero(s) borrados (metadatos conservados)\n", deleted)
	return 0
}
