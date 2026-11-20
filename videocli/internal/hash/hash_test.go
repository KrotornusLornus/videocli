package hash

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadStableAndSizeSensitive(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.bin")
	p2 := filepath.Join(dir, "b.bin")
	content := []byte("contenido de prueba muy largo para el hash")
	if err := os.WriteFile(p1, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p2, content, 0o644); err != nil {
		t.Fatal(err)
	}
	s1, sz1, err := Read(p1, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	s2, _, err := Read(p2, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	if s1 != s2 || sz1 != int64(len(content)) {
		t.Fatalf("contenidos iguales deberían dar hash igual: %s vs %s", s1, s2)
	}

	// Mismo prefijo, tamaño distinto → hash distinto.
	p3 := filepath.Join(dir, "c.bin")
	if err := os.WriteFile(p3, append(append([]byte{}, content...), 'x'), 0o644); err != nil {
		t.Fatal(err)
	}
	s3, _, err := Read(p3, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	if s1 == s3 {
		t.Fatal("tamaños distintos deberían dar hash distinto")
	}
}
