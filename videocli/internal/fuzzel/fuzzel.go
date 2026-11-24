// Package fuzzel integra el selector dinámico de fuzzel (dmenu).
package fuzzel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrCancelled se devuelve cuando el usuario cierra fuzzel sin elegir.
var ErrCancelled = errors.New("selección cancelada")

// Choose muestra items en fuzzel y devuelve el elegido.
//
// Con allowCustom=true, el texto tecleado que no coincide se devuelve tal cual
// (útil para crear etiquetas o teclear rutas). Con false se pasa
// --only-match y solo se puede elegir de la lista.
//
// prompt debe incluir el espacio final, p. ej. "Etiqueta> ".
func Choose(ctx context.Context, prompt string, items []string, allowCustom bool) (string, error) {
	if len(items) == 0 && !allowCustom {
		return "", fmt.Errorf("no hay opciones que mostrar")
	}
	args := []string{"--dmenu", "--prompt=" + prompt}
	if !allowCustom {
		args = append(args, "--only-match")
	}
	cmd := exec.CommandContext(ctx, "fuzzel", args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	for _, it := range items {
		if _, err := io.WriteString(stdin, it+"\n"); err != nil {
			return "", err
		}
	}
	if err := stdin.Close(); err != nil {
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", ErrCancelled
		}
		return "", err
	}
	line := strings.TrimRight(out.String(), "\n")
	if line == "" {
		return "", ErrCancelled
	}
	return line, nil
}

// Confirm muestra una pregunta con opciones y devuelve la elegida.
func Confirm(ctx context.Context, question string, options []string) (string, error) {
	return Choose(ctx, question+" > ", options, false)
}

// ChooseDir muestra las carpetas de start en fuzzel y devuelve la elegida
// directamente (el usuario selecciona la carpeta que quiere; para bajar más
// niveles, vuelve a invocarlo). Con ".." sube. roots añade accesos directos
// a otras carpetas de usuario desde cualquier nivel. ErrCancelled si se
// cancela.
func ChooseDir(ctx context.Context, start string, roots []string) (string, error) {
	entries, err := dirEntries(start, roots)
	if err != nil {
		return "", err
	}
	choice, err := Choose(ctx, fmt.Sprintf("Carpeta [%s] > ", start), entries, true)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(choice, "\t") {
		return "", fmt.Errorf("ruta no válida: %q", choice)
	}
	path := choice
	if path == "⬆ .." || path == ".." {
		path = "/"
		if start != "/" {
			path = filepath.Dir(start)
		}
	}
	if path == "" {
		return "", ErrCancelled
	}
	if strings.HasPrefix(path, "★ ") {
		path = strings.TrimPrefix(path, "★ ")
	}
	st, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("no existe: %s", path)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("no es una carpeta: %s", path)
	}
	return path, nil
}

func dirEntries(cur string, roots []string) ([]string, error) {
	var out []string
	if cur != "/" {
		out = append(out, "⬆ ..")
	}
	// Accesos directos a otras carpetas del usuario.
	for _, r := range roots {
		if r != cur {
			out = append(out, "★ "+r)
		}
	}
	file, err := os.Open(cur)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	names, err := file.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	for _, n := range names {
		if strings.HasPrefix(n, ".") {
			continue
		}
		st, err := os.Stat(cur + "/" + n)
		if err != nil || !st.IsDir() {
			continue
		}
		out = append(out, cur+"/"+n)
	}
	return out, nil
}
