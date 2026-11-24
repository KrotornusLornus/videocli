package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"videocli/internal/db"
	"videocli/internal/fuzzel"
	"videocli/internal/player"
	"videocli/internal/thumb"
)

// resolveVideo interpreta un argumento como id numérico, ruta de vídeo
// o ruta de miniatura.
func resolveVideo(c *ctx, ref string) (*db.Video, error) {
	if ref == "" {
		return nil, db.ErrNotFound
	}
	if id, err := parseID(ref); err == nil {
		if v, err := c.dbr.GetVideo(id); err == nil {
			return v, nil
		}
	}
	return c.dbr.FindByAnyPath(ref)
}

func parseID(s string) (int64, error) {
	var id int64
	if _, err := fmt.Sscanf(s, "%d", &id); err != nil || id <= 0 {
		return 0, fmt.Errorf("no es un id")
	}
	return id, nil
}

func cmdThumb(c *ctx) int {
	// Parseo manual: flag acepta --regenerate antes o después de la ruta.
	regen := ""
	var rest []string
	for i := 0; i < len(c.args); i++ {
		switch {
		case c.args[i] == "--regenerate" || c.args[i] == "-regenerate":
			if i+1 < len(c.args) {
				regen = c.args[i+1]
				i++
			}
		case strings.HasPrefix(c.args[i], "--regenerate="):
			regen = strings.TrimPrefix(c.args[i], "--regenerate=")
		case strings.HasPrefix(c.args[i], "-regenerate="):
			regen = strings.TrimPrefix(c.args[i], "-regenerate=")
		default:
			rest = append(rest, c.args[i])
		}
	}
	if len(rest) != 1 {
		return fail(2, "uso: videocli thumb <ruta|id> [--regenerate local|online]")
	}
	v, err := resolveVideo(c, rest[0])
	if err != nil {
		return fail(1, "vídeo no encontrado: %s", rest[0])
	}
	// Siempre en la ubicación canónica (las entradas antiguas pueden tener
	// rutas previas a migraciones de configuración).
	thumbPath := filepath.Join(c.cfg.Paths.ThumbsDir(), fmt.Sprintf("%d.jpg", v.ID))
	method := strings.TrimSpace(regen)
	if method == "" {
		if _, statErr := os.Stat(v.Path); statErr == nil {
			method = "local"
		} else if v.URL != nil {
			method = "online"
		}
	}
	opts := thumb.Options{Width: c.cfg.Thumb.Width, Quality: c.cfg.Thumb.Quality, Seek: c.cfg.Thumb.Seek}
	if method == "" {
		// Sin fuente (fichero local borrado y sin URL): placeholder para
		// que el registro siga visible en la galería.
		if err := thumb.GeneratePlaceholder(context.Background(), thumbPath, opts); err != nil {
			return fail(1, "%v", err)
		}
		if err := c.dbr.UpdateVideoThumb(v.ID, thumbPath); err != nil {
			return fail(1, "%v", err)
		}
		if err := c.dbr.UpdateVideoStatus(v.ID, v.Status, "sin fuente para la miniatura"); err != nil {
			return fail(1, "%v", err)
		}
		fmt.Fprintln(os.Stderr, "videocli: placeholder generado (sin fuente)")
		fmt.Println(thumbPath)
		return 0
	}
	if err := thumb.Generate(context.Background(), method, v.Path, derefURL(v.URL), thumbPath, opts); err != nil {
		return fail(1, "%v", err)
	}
	// Actualiza la miniatura y limpia el aviso de miniatura pendiente.
	if err := c.dbr.UpdateVideoThumb(v.ID, thumbPath); err != nil {
		return fail(1, "%v", err)
	}
	if err := c.dbr.UpdateVideoStatus(v.ID, v.Status, ""); err != nil {
		return fail(1, "%v", err)
	}
	fmt.Println(thumbPath)
	return 0
}

// fileExists comprueba que exista y no sea un directorio.
func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func derefURL(u *string) string {
	if u == nil {
		return ""
	}
	return *u
}

func cmdPlay(c *ctx) int {
	// Parseo manual (flags antes o después de las rutas).
	music, separate, shuffle := false, false, false
	var refs []string
	for _, a := range c.args {
		switch a {
		case "--music", "-music":
			music = true
		case "--separate", "-separate":
			separate = true
		case "--shuffle", "-shuffle":
			shuffle = true
		default:
			refs = append(refs, a)
		}
	}
	if len(refs) == 0 {
		return fail(2, "uso: videocli play RUTA... [--music] [--separate] [--shuffle]")
	}
	if music && separate {
		// El modo música es siempre una única cola de mpv.
		return fail(2, "el modo música no admite procesos separados")
	}
	// Audio en instancia única: sustituye la cola de música anterior.
	var extra []string
	if music {
		const marker = "videocli-audio"
		if pkill, err := exec.LookPath("pkill"); err == nil {
			_ = exec.Command(pkill, "-f", marker).Run()
		}
		extra = []string{"--force-media-title=" + marker}
	}
	if shuffle {
		extra = append(extra, "--shuffle")
	}
	var paths []string
	for _, ref := range refs {
		if v, err := resolveVideo(c, ref); err == nil {
			var path string
			switch {
			case v.Path != "" && fileExists(v.Path):
				path = v.Path
			case v.URL != nil && *v.URL != "":
				// Sin fichero local (o desaparecido): streaming por URL.
				if v.Path != "" {
					fmt.Fprintf(os.Stderr, "videocli: fichero local no disponible (%s); reproduciendo en streaming\n", v.Path)
				}
				path = *v.URL
			default:
				// No asumimos que el registro ya no vale: puede volver con
				// un re-escaneo si el fichero se movió.
				fmt.Fprintf(os.Stderr, "videocli: no se encuentra el vídeo %q (el registro se conserva; re-escanea si se movió)\n", v.Title)
				continue
			}
			paths = append(paths, path)
			continue
		}
		if _, statErr := os.Stat(ref); statErr == nil {
			// Ruta arbitraria: reproducirla directamente.
			abs, _ := filepath.Abs(ref)
			paths = append(paths, abs)
		} else {
			fmt.Fprintf(os.Stderr, "videocli: vídeo no encontrado: %s\n", ref)
		}
	}
	if len(paths) == 0 {
		return fail(1, "no se encontró ningún vídeo reproducible (los registros se conservan)")
	}
	if separate {
		// Lanza un reproductor independiente por vídeo.
		for _, p := range paths {
			if err := player.Play(c.cfg.Player.Command, c.cfg.Player.Args,
				c.cfg.Player.MusicArgs, p, music, extra...); err != nil {
				return fail(1, "%v", err)
			}
		}
		return 0
	}
	// Un solo reproductor con cola (playlist).
	if err := player.PlayAll(c.cfg.Player.Command, c.cfg.Player.Args,
		c.cfg.Player.MusicArgs, paths, music, extra...); err != nil {
		return fail(1, "%v", err)
	}
	return 0
}

// categoryUsage muestra la ayuda del subcomando category.
func categoryUsage() {
	fmt.Fprint(os.Stderr, `uso: videocli category <subcomando>

  list                    lista categorías con uso
  create RUTA             crea una categoría (ruta tipo directorio)
  delete RUTA             borra una categoría (los vídeos quedan sin ella)
  add VIDEO RUTA          asocia (y crea si hace falta) al vídeo
  remove VIDEO RUTA       desasocia del vídeo
  toggle VIDEO...         selector fuzzel para añadir/sacar el vídeo de
                          categorías (con varios vídeos, solo las comunes)
  manage                  selector fuzzel para gestionar categorías globales

  La categoría especial "sin-categoria" es virtual: solo aparecen en ella
  los vídeos sin ninguna categoría asignada.
`)
}

func cmdCategory(c *ctx) int {
	if len(c.args) == 0 {
		categoryUsage()
		return 2
	}
	sub, rest := c.args[0], c.args[1:]
	switch sub {
	case "list":
		cats, err := c.dbr.ListCategories()
		if err != nil {
			return fail(1, "%v", err)
		}
		for _, ct := range cats {
			fmt.Printf("%s\t%d\n", ct.Path, ct.Count)
		}
		return 0
	case "create":
		if len(rest) != 1 {
			return fail(2, "uso: videocli category create RUTA")
		}
		if _, err := c.dbr.EnsureCategory(rest[0]); err != nil {
			return fail(1, "%v", err)
		}
		return 0
	case "delete":
		if len(rest) != 1 {
			return fail(2, "uso: videocli category delete RUTA")
		}
		if err := c.dbr.DeleteCategory(rest[0]); err != nil {
			return fail(1, "%v", err)
		}
		return 0
	case "add", "remove":
		if len(rest) != 2 {
			return fail(2, "uso: videocli category %s VIDEO RUTA", sub)
		}
		v, err := resolveVideo(c, rest[0])
		if err != nil {
			return fail(1, "vídeo no encontrado: %s", rest[0])
		}
		catID, err := c.dbr.EnsureCategory(rest[1])
		if err != nil {
			return fail(1, "%v", err)
		}
		if sub == "add" {
			err = c.dbr.AssignCategory(v.ID, catID)
		} else {
			err = c.dbr.UnassignCategory(v.ID, catID)
		}
		if err != nil {
			return fail(1, "%v", err)
		}
		return 0
	case "toggle":
		return categoryToggle(c, rest)
	case "manage":
		return categoryManage(c)
	default:
		categoryUsage()
		return 2
	}
}

// categoryToggle abre un selector de casillas: las categorías comunes a
// TODOS los vídeos seleccionados van marcadas [x] (elegirla las saca) y el
// resto [ ] (elegirla las añade). Bucle hasta Escape.
func categoryToggle(c *ctx, args []string) int {
	if len(args) == 0 {
		return fail(2, "uso: videocli category toggle VIDEO...")
	}
	var videos []*db.Video
	for _, ref := range args {
		v, err := resolveVideo(c, ref)
		if err != nil {
			return fail(1, "vídeo no encontrado: %s", ref)
		}
		videos = append(videos, v)
	}
	for {
		// Categorías que tienen TODOS los vídeos seleccionados.
		counts := map[string]int{}
		for _, v := range videos {
			names, err := c.dbr.ListVideoCategories(v.ID)
			if err != nil {
				return fail(1, "%v", err)
			}
			seen := map[string]bool{}
			for _, name := range names {
				if !seen[name] {
					counts[name]++
					seen[name] = true
				}
			}
		}
		allCats, err := c.dbr.ListCategories()
		if err != nil {
			return fail(1, "%v", err)
		}
		items := make([]string, 0, len(allCats))
		for _, cat := range allCats {
			if counts[cat.Path] == len(videos) {
				items = append(items, "[x] "+cat.Path)
			} else {
				items = append(items, "[ ] "+cat.Path)
			}
		}
		choice, perr := fuzzel.Choose(context.Background(),
			"Categoría (añadir/sacar) > ", items, false)
		if perr != nil {
			return 0
		}
		assigned := strings.HasPrefix(choice, "[x]")
		path := strings.TrimPrefix(choice, "[x] ")
		path = strings.TrimPrefix(path, "[ ] ")
		catID, err := c.dbr.GetCategory(path)
		if err != nil {
			continue
		}
		for _, v := range videos {
			if assigned {
				_ = c.dbr.UnassignCategory(v.ID, catID)
			} else {
				_ = c.dbr.AssignCategory(v.ID, catID)
			}
		}
	}
}

// categoryManage permite crear y borrar categorías globales con fuzzel.
func categoryManage(c *ctx) int {
	cats, err := c.dbr.ListCategories()
	if err != nil {
		return fail(1, "%v", err)
	}
	items := make([]string, 0, len(cats))
	for _, ct := range cats {
		items = append(items, fmt.Sprintf("%s (%d)", ct.Path, ct.Count))
	}
	choice, err := fuzzel.Choose(context.Background(), "Categoría (escribe para crear) > ", items, true)
	if err != nil {
		return 0
	}
	name := strings.TrimSpace(choice)
	if strings.HasSuffix(name, ")") {
		if i := strings.LastIndex(name, " ("); i > 0 {
			name = name[:i]
		}
	}
	if name == "" {
		return 0
	}
	if _, err := c.dbr.GetCategory(name); err == db.ErrNotFound {
		if _, err := c.dbr.EnsureCategory(name); err != nil {
			return fail(1, "%v", err)
		}
		fmt.Fprintf(os.Stderr, "videocli: categoría %q creada\n", name)
		return 0
	}
	// Existente: renombrar (cascada), borrar o conservar.
	action, err := fuzzel.Confirm(context.Background(), fmt.Sprintf("%s:", name),
		[]string{"conservar", "renombrar", "borrar"})
	if err != nil || action == "conservar" {
		return 0
	}
	if action == "borrar" {
		if err := c.dbr.DeleteCategory(name); err != nil {
			return fail(1, "%v", err)
		}
		fmt.Fprintf(os.Stderr, "videocli: categoría %q borrada\n", name)
		return 0
	}
	// Renombrar: nueva ruta con texto libre.
	newPath, err := fuzzel.Choose(context.Background(),
		fmt.Sprintf("Nueva ruta para %s > ", name), nil, true)
	if err != nil {
		return 0
	}
	if err := c.dbr.RenameCategory(name, newPath); err != nil {
		return fail(1, "%v", err)
	}
	fmt.Fprintf(os.Stderr, "videocli: categoría %q renombrada a %q (cascada)\n", name, newPath)
	return 0
}
