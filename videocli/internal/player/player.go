// Package player lanza el reproductor (mpv) de forma desatendida.
package player

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// PlayAll lanza command con una cola (playlist) de ficheros: un solo proceso
// que reproduce paths en orden.
func PlayAll(command string, args, musicArgs []string, paths []string, music bool) error {
	if len(paths) == 0 {
		return nil
	}
	return launch(command, args, musicArgs, paths, music)
}

// Play lanza command con un único fichero.
func Play(command string, args, musicArgs []string, videoPath string, music bool) error {
	return launch(command, args, musicArgs, []string{videoPath}, music)
}

func launch(command string, args, musicArgs []string, paths []string, music bool) error {
	if _, err := exec.LookPath(command); err != nil {
		return fmt.Errorf("reproductor %q no disponible: %w", command, err)
	}
	full := append([]string{}, args...)
	if music {
		full = append(full, musicArgs...)
	}
	full = append(full, "--")
	full = append(full, paths...)

	cmd := exec.Command(command, full...)
	cmd.Stdin = nil
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err == nil {
		cmd.Stdout = devnull
		cmd.Stderr = devnull
		defer devnull.Close()
	}
	// Nueva sesión para que el reproductor no muera con el CLI.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	// No esperamos: el proceso queda huérfano reparentado a init/PID1.
	go func() { _ = cmd.Wait() }()
	return nil
}
