// Package cli implementa los subcomandos de videocli.
package cli

import (
	"fmt"
	"os"
	"strings"

	"videocli/internal/config"
	"videocli/internal/db"
)

// version es la versión informada por `videocli version`.
const version = "0.1.0"

// Run ejecuta el subcomando indicado y devuelve el código de salida.
func Run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "doctor":
		return cmdDoctor(rest)
	case "scan":
		return withDB(rest, cmdScan)
	case "list":
		return withDB(rest, cmdList)
	case "thumb":
		return withDB(rest, cmdThumb)
	case "play":
		return withDB(rest, cmdPlay)
	case "categories":
		return withDB(rest, cmdCategories)
	case "pick-dir":
		return withDB(rest, cmdPickDir)
	case "import":
		return withDB(rest, cmdImport)
	case "import-run":
		return withDB(rest, cmdImportRun)
	case "fetch":
		return withDB(rest, cmdFetch)
	case "fetch-run":
		return withDB(rest, cmdFetchRun)
	case "category":
		return withDB(rest, cmdCategory)
	case "info":
		return withDB(rest, cmdInfo)
	case "remove":
		return withDB(rest, cmdRemove)
	case "delete-file":
		return withDB(rest, cmdDeleteFile)
	case "url":
		return withDB(rest, cmdUrl)
	case "locate":
		return withDB(rest, cmdLocate)
	case "config":
		return cmdConfig(rest)
	case "version":
		fmt.Printf("videocli %s\n", version)
		return 0
	case "help", "--help", "-h":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "videocli: subcomando desconocido: %s\n", cmd)
		usage()
		return 2
	}
}

type cmdFunc func(c *ctx) int

// ctx agrupa lo que necesitan los subcomandos.
type ctx struct {
	cfg  config.Config
	dbr  *db.DB
	args []string
}

// withDB carga config y abre la DB, y ejecuta fn.
func withDB(args []string, fn cmdFunc) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "videocli: %v\n", err)
		return 1
	}
	database, err := db.Open(cfg.Paths.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "videocli: %v\n", err)
		return 1
	}
	defer database.Close()
	return fn(&ctx{cfg: cfg, dbr: database, args: args})
}

func usage() {
	fmt.Print(`videocli - gestor de videoteca para swayimg

Uso:
  videocli <subcomando> [opciones]

Subcomandos:
  doctor               comprueba dependencias y estado
  scan [DIR] [--fast]  registra vídeos (sin DIR usa el selector fuzzel)
  list [--tag T] [--untagged] [--json]   lista miniaturas para la galería
  thumb PATH [--regenerate local|online] genera o regenera una miniatura
  play PATH [--music]  reproduce con mpv (PATH puede ser la miniatura)
  categories           lista categorías
  pick-dir             selector fuzzel de carpetas (empieza en ~/Videos)
  import [URL...]      registra URLs con solo metadatos (sin args abre vi)
  fetch RUTA|ID        descarga el vídeo de una entrada importada
  category list|create|delete|add|remove|toggle|manage
  remove RUTA|ID...     borra los metadatos de la base de datos
  delete-file RUTA|ID...  borra el fichero de vídeo local (metadatos no)
  url RUTA|ID          imprime la URL de origen (vacío si no tiene)
  locate RUTA|ID       imprime la ruta local del vídeo (vacío si no tiene)
  info PATH            resumen del vídeo (para la galería)
  config               muestra la ruta del fichero de configuración
  version              muestra la versión

Toda la información va a stderr; stdout queda limpio para swayimg.
`)
}

// loadConfig es el cargador usado fuera de withDB.
func loadConfig() (config.Config, int) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "videocli: %v\n", err)
		return cfg, 1
	}
	return cfg, 0
}

// fail escribe un error a stderr y devuelve el código indicado.
func fail(code int, format string, args ...any) int {
	msg := strings.TrimSuffix(fmt.Sprintf(format, args...), "\n")
	fmt.Fprintf(os.Stderr, "videocli: %s\n", msg)
	return code
}
