package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"videocli/internal/db"
	"videocli/internal/download"
	"videocli/internal/fuzzel"
)

// viBuffer abre vi en $TERMINAL (o foot) sobre un fichero temporal y
// devuelve las líneas útiles (no vacías ni comentarios).
func viBuffer(header string) ([]string, error) {
	tmp, err := os.CreateTemp(os.TempDir(), "videocli-urls-*.txt")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(header); err != nil {
		tmp.Close()
		return nil, err
	}
	tmp.Close()

	term := os.Getenv("TERMINAL")
	if term == "" {
		term = "foot"
	}
	// Ejecuta el terminal de forma síncrona: se cierra al guardar y salir de vi.
	cmd := exec.Command(term, "-e", "vi", tmp.Name())
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("editor: %w", err)
	}
	f, err := os.Open(tmp.Name())
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines, sc.Err()
}

// cmdImport registra URLs con solo sus metadatos (sin descargar el vídeo).
// Sin argumentos abre vi con una URL por línea.
func cmdImport(c *ctx) int {
	var urls []string
	if len(c.args) > 0 {
		urls = c.args
	}
	if len(urls) == 0 {
		header := "# Una URL de vídeo por línea.\n# Las líneas vacías o que empiezan por # se ignoran.\n# Guarda y cierra (:wq) para importar.\n"
		lines, err := viBuffer(header)
		if err != nil {
			return fail(1, "%v", err)
		}
		urls = lines
	}
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "videocli: sin URLs que importar")
		return 0
	}
	imported := 0
	for _, url := range urls {
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			fmt.Fprintf(os.Stderr, "videocli: no parece una URL, se ignora: %s\n", url)
			continue
		}
		v := &db.Video{URL: &url, Source: "url", Status: "pending"}
		id, err := c.dbr.InsertVideo(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "videocli: %v\n", err)
			continue
		}
		if err := spawnWorker("import-run", fmt.Sprintf("%d", id)); err != nil {
			fmt.Fprintf(os.Stderr, "videocli: %v\n", err)
			continue
		}
		imported++
	}
	fmt.Fprintf(os.Stderr, "videocli: %d URLs en cola de importación\n", imported)
	return 0
}

// spawnWorker lanza videocli con los argumentos dados (setsid, segundo plano).
func spawnWorker(args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	worker := exec.Command(exe, args...)
	worker.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := worker.Start(); err != nil {
		return err
	}
	go func() { _ = worker.Wait() }()
	return nil
}

func cmdImportRun(c *ctx) int {
	if len(c.args) != 1 {
		return fail(2, "uso interno: videocli import-run <id>")
	}
	var id int64
	if _, err := fmt.Sscanf(c.args[0], "%d", &id); err != nil {
		return fail(2, "id no válido: %s", c.args[0])
	}
	if err := download.Import(context.Background(), c.dbr, c.cfg, id); err != nil {
		return fail(1, "%v", err)
	}
	return 0
}

// cmdFetch descarga el vídeo de una entrada importada (solo metadatos).
func cmdFetch(c *ctx) int {
	if len(c.args) != 1 {
		return fail(2, "uso: videocli fetch <ruta|id>")
	}
	v, err := resolveVideo(c, c.args[0])
	if err != nil {
		return fail(1, "vídeo no encontrado: %s", c.args[0])
	}
	if v.URL == nil {
		return fail(1, "el vídeo no tiene URL de origen")
	}
	if v.Path != "" {
		if fileExists(v.Path) {
			return fail(1, "ya está descargado: %s", v.Path)
		}
		// El fichero ya no existe en disco: permitir re-descarga.
		fmt.Fprintf(os.Stderr, "videocli: fichero local no disponible (%s); re-descargando\n", v.Path)
	}
	if err := c.dbr.UpdateVideoStatus(v.ID, "downloading", ""); err != nil {
		return fail(1, "%v", err)
	}
	// Elegir calidad máxima con fuzzel (0 = sin límite).
	qualities := append([]string{"mejor disponible"},
		intsToQualityLabels(c.cfg.Download.Qualities)...)
	choice, ferr := fuzzel.Choose(context.Background(), "Calidad máxima > ", qualities, false)
	if ferr != nil {
		return fail(1, "descarga cancelada")
	}
	maxH := qualityLabelToHeight(choice)

	// Elegir carpeta de destino: primero las de usuario, luego navegador.
	opts := []string{c.cfg.Paths.DownloadDir}
	opts = append(opts, c.cfg.Scan.Roots...)
	opts = append(opts, "elegir carpeta…")
	dest, derr := fuzzel.Choose(context.Background(), "Destino > ", opts, true)
	if derr != nil {
		return fail(1, "descarga cancelada")
	}
	if dest == "elegir carpeta…" {
		dest, derr = fuzzel.ChooseDir(context.Background(), scanStartDir(c.cfg), c.cfg.Scan.Roots)
		if derr != nil {
			return fail(1, "descarga cancelada")
		}
	}
	if !statIsDir(dest) {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return fail(1, "no se pudo crear el destino %s: %v", dest, err)
		}
	}

	if err := spawnWorker("fetch-run", fmt.Sprintf("%d", v.ID),
		fmt.Sprintf("%d", maxH), dest); err != nil {
		return fail(1, "%v", err)
	}
	fmt.Fprintf(os.Stderr, "videocli: descarga en curso (id %d, calidad ≤ %s, destino %s)\n",
		v.ID, choice, dest)
	return 0
}

// intsToQualityLabels convierte alturas a etiquetas legibles ("1080p").
func intsToQualityLabels(qualities []int) []string {
	out := make([]string, 0, len(qualities))
	for _, q := range qualities {
		out = append(out, fmt.Sprintf("%dp", q))
	}
	return out
}

// qualityLabelToHeight interpreta la etiqueta elegida ("1080p" → 1080).
func qualityLabelToHeight(label string) int64 {
	label = strings.TrimSuffix(strings.TrimSpace(label), "p")
	var h int64
	if _, err := fmt.Sscanf(label, "%d", &h); err != nil {
		return 0
	}
	return h
}

func cmdFetchRun(c *ctx) int {
	if len(c.args) != 1 && len(c.args) != 2 && len(c.args) != 3 {
		return fail(2, "uso interno: videocli fetch-run <id> [altura-máxima] [destino]")
	}
	var id, maxH int64
	if _, err := fmt.Sscanf(c.args[0], "%d", &id); err != nil {
		return fail(2, "id no válido: %s", c.args[0])
	}
	if len(c.args) >= 2 {
		if _, err := fmt.Sscanf(c.args[1], "%d", &maxH); err != nil {
			return fail(2, "altura no válida: %s", c.args[1])
		}
	}
	dest := ""
	if len(c.args) == 3 {
		dest = expandTilde(c.args[2])
	}
	if err := download.Fetch(context.Background(), c.dbr, c.cfg, id, maxH, dest); err != nil {
		return fail(1, "%v", err)
	}
	return 0
}
