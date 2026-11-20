package main

import (

	"os"

	"videocli/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
