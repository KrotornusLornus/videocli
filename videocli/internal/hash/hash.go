// Package hash calcula huellas de contenido para detectar ficheros
// repetidos o movidos.
package hash

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
)

// Read computes the SHA-256 of the first n bytes of the file plus its size,
// so two files with identical prefixes but different sizes hash differently.
func Read(path string, n int64) (sum string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	var sz [8]byte
	binary.LittleEndian.PutUint64(sz[:], uint64(st.Size()))
	h.Write(sz[:])
	if _, err := io.CopyN(h, f, n); err != nil && err != io.EOF {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), st.Size(), nil
}
