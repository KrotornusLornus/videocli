package cli

import (
	"fmt"
	"os"

	"videocli/internal/hash"
)

// reconcileMissing comprueba que cada registro con fichero local siga
// existiendo en disco. Los que ya no están pierden la ruta (los metadatos
// se conservan; si tienen URL vuelven a estado url) y reciben placeholder
// de miniatura si falta. Devuelve cuántos se reconciliaron.
func reconcileMissing(c *ctx) int {
	videos, err := c.dbr.AllVideos()
	if err != nil {
		return 0
	}
	fixed, hashed := 0, 0
	for _, v := range videos {
		// Hash pendiente en registros con fichero existente (p. ej. añadidos
		// por descarga antes de este arreglo).
		if v.Path != "" && v.ContentHash == "" {
			if _, statErr := os.Stat(v.Path); statErr == nil {
				if sum, size, herr := hash.Read(v.Path, 64*1024); herr == nil {
					_ = c.dbr.UpdateVideoHash(v.ID, sum, size)
					hashed++
				}
			}
		}
		if v.Path == "" {
			continue
		}
		if _, statErr := os.Stat(v.Path); !os.IsNotExist(statErr) {
			continue
		}
		// El fichero ya no está: quitar la ruta del registro.
		if err := c.dbr.ClearVideoPath(v.ID); err != nil {
			continue
		}
		note := "fichero local eliminado"
		status := v.Status
		if v.URL != nil && *v.URL != "" {
			// Con URL de origen puede re-descargarse (Ctrl+d).
			status = "url"
		}
		_ = c.dbr.UpdateVideoStatus(v.ID, status, note)
		// Si el fichero de miniatura desapareció, limpiar la ruta: el
		// registro queda oculto hasta regeneración manual (tecla r).
		if v.ThumbPath != "" {
			if _, statErr := os.Stat(v.ThumbPath); os.IsNotExist(statErr) {
				_ = c.dbr.UpdateVideoThumb(v.ID, "")
			}
		}
		fixed++
	}
	if hashed > 0 {
		fmt.Fprintf(os.Stderr, "videocli: hash rellenado para %d registro(s)\n", hashed)
	}
	return fixed
}

// cmdVerify reconcilia la base de datos con el disco: rutas de ficheros
// que ya no existen pierden la ruta (metadatos conservados).
func cmdVerify(c *ctx) int {
	fixed := reconcileMissing(c)
	if fixed == 0 {
		fmt.Fprintln(os.Stderr, "videocli: todo en orden, ningún fichero perdido")
		return 0
	}
	fmt.Fprintf(os.Stderr, "videocli: %d registro(s) reconciliados (ficheros ya no presentes)\n", fixed)
	return 0
}

// logMissing avisa por stderr de lo reconciliado tras un escaneo.
func logMissing(c *ctx) {
	if fixed := reconcileMissing(c); fixed > 0 {
		fmt.Fprintf(os.Stderr, "videocli: %d registro(s) con ficheros desaparecidos reconciliados\n", fixed)
	}
}
