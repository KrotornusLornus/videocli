package db

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Video es un registro de la tabla videos.
type Video struct {
	ID          int64
	Title       string
	URL         *string
	Path        string
	ThumbPath   string
	ContentHash string
	HashSize    int64
	DurationMS  int64
	Width       int
	Height      int
	VideoCodec  string
	AudioCodec  string
	Status      string
	Error       string
	Source      string
	CreatedAt   string
	UpdatedAt   string
}

func scanVideo(row interface{ Scan(...any) error }) (*Video, error) {
	var v Video
	var url any
	err := row.Scan(&v.ID, &v.Title, &url, &v.Path, &v.ThumbPath, &v.ContentHash,
		&v.HashSize, &v.DurationMS, &v.Width, &v.Height, &v.VideoCodec,
		&v.AudioCodec, &v.Status, &v.Error, &v.Source, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if url != nil {
		s := url.(string)
		v.URL = &s
	}
	return &v, nil
}

const videoCols = `id, title, url, COALESCE(path, '') AS path, thumb_path,
content_hash, hash_size, duration_ms, width, height, video_codec, audio_codec,
status, error, source, created_at, updated_at`

// InsertVideo inserta v y devuelve su id. Deja created_at/updated_at en now.
func (d *DB) InsertVideo(v *Video) (int64, error) {
	if v.Status == "" {
		v.Status = "ok"
	}
	if v.Source == "" {
		v.Source = "local"
	}
	ts := now()
	v.CreatedAt, v.UpdatedAt = ts, ts
	var url, path any
	if v.URL != nil {
		url = *v.URL
	}
	if v.Path != "" {
		path = v.Path
	}
	res, err := d.db.Exec(`INSERT INTO videos (title, url, path, thumb_path,
		content_hash, hash_size, status, source, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		v.Title, url, path, v.ThumbPath, v.ContentHash, v.HashSize,
		v.Status, v.Source, v.CreatedAt, v.UpdatedAt)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	v.ID = id
	return id, nil
}

// GetVideo devuelve el vídeo con id dado o ErrNotFound.
func (d *DB) GetVideo(id int64) (*Video, error) {
	v, err := scanVideo(d.db.QueryRow(
		`SELECT `+videoCols+` FROM videos WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

// FindByPath devuelve el vídeo cuya ruta de fichero es path.
func (d *DB) FindByPath(path string) (*Video, error) {
	v, err := scanVideo(d.db.QueryRow(
		`SELECT `+videoCols+` FROM videos WHERE path = ?`, path))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

// GetByThumbPath devuelve el vídeo cuya miniatura es path.
func (d *DB) GetByThumbPath(path string) (*Video, error) {
	v, err := scanVideo(d.db.QueryRow(
		`SELECT `+videoCols+` FROM videos WHERE thumb_path = ?`, path))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

// FindByAnyPath resuelve una ruta que puede ser el vídeo o su miniatura.
func (d *DB) FindByAnyPath(path string) (*Video, error) {
	v, err := scanVideo(d.db.QueryRow(
		`SELECT `+videoCols+` FROM videos WHERE path = ? OR thumb_path = ?`,
		path, path))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

// FindByHash busca un vídeo con el mismo hash y tamaño.
func (d *DB) FindByHash(hash string, size int64) (*Video, error) {
	v, err := scanVideo(d.db.QueryRow(
		`SELECT `+videoCols+` FROM videos WHERE content_hash = ? AND hash_size = ?`,
		hash, size))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

// UpdateVideoPath reubica un vídeo (fichero movido).
func (d *DB) UpdateVideoPath(id int64, path string) error {
	_, err := d.db.Exec(`UPDATE videos SET path = ?, updated_at = ? WHERE id = ?`,
		path, now(), id)
	return err
}

// UpdateVideoThumb fija la ruta de miniatura.
func (d *DB) UpdateVideoThumb(id int64, thumb string) error {
	_, err := d.db.Exec(`UPDATE videos SET thumb_path = ?, updated_at = ? WHERE id = ?`,
		thumb, now(), id)
	return err
}

// UpdateVideoMeta guarda duración, resolución y códecs.
func (d *DB) UpdateVideoMeta(id int64, durationMS int64, w, h int, videoCodec, audioCodec string) error {
	_, err := d.db.Exec(`UPDATE videos SET duration_ms = ?, width = ?, height = ?,
		video_codec = ?, audio_codec = ?, updated_at = ? WHERE id = ?`,
		durationMS, w, h, videoCodec, audioCodec, now(), id)
	return err
}

// UpdateVideoTitle fija el título.
func (d *DB) UpdateVideoTitle(id int64, title string) error {
	_, err := d.db.Exec(`UPDATE videos SET title = ?, updated_at = ? WHERE id = ?`,
		title, now(), id)
	return err
}

// UpdateVideoStatus fija estado y error.
func (d *DB) UpdateVideoStatus(id int64, status, errMsg string) error {
	_, err := d.db.Exec(`UPDATE videos SET status = ?, error = ?, updated_at = ? WHERE id = ?`,
		status, errMsg, now(), id)
	return err
}

// UpdateVideoSource fija el origen (local|download).
func (d *DB) UpdateVideoSource(id int64, source string) error {
	_, err := d.db.Exec(`UPDATE videos SET source = ?, updated_at = ? WHERE id = ?`,
		source, now(), id)
	return err
}

// DeleteVideo elimina un vídeo y sus asociaciones.
func (d *DB) DeleteVideo(id int64) error {
	_, err := d.db.Exec(`DELETE FROM videos WHERE id = ?`, id)
	return err
}

// ListVideos devuelve vídeos filtrados, ordenados por creación descendente.
// Con category != "" filtra por categoría con herencia de prefijo
// (categoria "musica" incluye "musica/80s"); el valor especial
// "sin-categoria" devuelve los vídeos sin ninguna categoría.
// Con dir != "", solo vídeos bajo ese directorio.
// Los vídeos sin miniatura se excluyen (no son visibles en la galería).
func (d *DB) ListVideos(category string, dir string) ([]*Video, error) {
	q := `SELECT ` + videoCols + ` FROM videos v WHERE v.thumb_path <> ''`
	args := []any{}
	if category != "" {
		if category == SinCategoria {
			q += ` AND NOT EXISTS (SELECT 1 FROM video_categories vt WHERE vt.video_id = v.id)`
		} else {
			// Herencia estilo directorio: ruta igual o subcategoría (X/...).
			q += ` AND EXISTS (SELECT 1 FROM video_categories vt JOIN categories c ON c.id = vt.category_id
				WHERE vt.video_id = v.id AND (c.path = ? COLLATE NOCASE OR substr(c.path, 1, ?) = ? COLLATE NOCASE))`
			args = append(args, category, int64(len(category)+1), category+"/")
		}
	}
	if dir != "" {
		// Comparación por prefijo exacto (dir + "/"), sin sorpresas de LIKE.
		q += ` AND substr(v.path, 1, ?) = ?`
		prefix := dir + "/"
		args = append(args, int64(len(prefix)))
		args = append(args, prefix)
	}
	q += ` ORDER BY v.created_at DESC, v.id DESC`
	rows, err := d.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Video
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Dirs devuelve los directorios que contienen vídeos visibles.
func (d *DB) Dirs() ([]string, error) {
	rows, err := d.db.Query(`SELECT path FROM videos WHERE thumb_path <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		dir := filepath.Dir(path)
		seen[dir] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(seen))
	for dir := range seen {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs, nil
}

// --- Etiquetas ---

// --- Categorías (rutas estilo directorio) ---

// sinCategoria es la categoría virtual de los vídeos sin ninguna.
const SinCategoria = "sin-categoria"

// EnsureCategory devuelve el id de la categoría, creándola si no existe.
// Normaliza la ruta: recorta espacios y barras de los extremos.
func (d *DB) EnsureCategory(path string) (int64, error) {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" || path == SinCategoria {
		return 0, fmt.Errorf("categoría no válida: %q", path)
	}
	if _, err := d.db.Exec(
		`INSERT INTO categories (path) VALUES (?) ON CONFLICT(path) DO NOTHING`, path); err != nil {
		return 0, err
	}
	var id int64
	err := d.db.QueryRow(`SELECT id FROM categories WHERE path = ? COLLATE NOCASE`, path).Scan(&id)
	return id, err
}

// CategoryCount es una categoría con el número de vídeos que la usan.
type CategoryCount struct {
	ID    int64
	Path  string
	Count int64
}

// ListCategories devuelve todas las categorías con su uso, ordenadas por ruta.
func (d *DB) ListCategories() ([]CategoryCount, error) {
	rows, err := d.db.Query(`SELECT c.id, c.path, COUNT(vt.video_id)
		FROM categories c LEFT JOIN video_categories vt ON vt.category_id = c.id
		GROUP BY c.id, c.path ORDER BY c.path COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CategoryCount
	for rows.Next() {
		var cc CategoryCount
		if err := rows.Scan(&cc.ID, &cc.Path, &cc.Count); err != nil {
			return nil, err
		}
		out = append(out, cc)
	}
	return out, rows.Err()
}

// AssignCategory asocia una categoría a un vídeo (idempotente).
func (d *DB) AssignCategory(videoID, categoryID int64) error {
	_, err := d.db.Exec(`INSERT INTO video_categories (video_id, category_id) VALUES (?, ?)
		ON CONFLICT DO NOTHING`, videoID, categoryID)
	return err
}

// UnassignCategory desasocia una categoría de un vídeo.
func (d *DB) UnassignCategory(videoID, categoryID int64) error {
	_, err := d.db.Exec(`DELETE FROM video_categories WHERE video_id = ? AND category_id = ?`,
		videoID, categoryID)
	return err
}

// ListVideoCategories devuelve las rutas de las categorías de un vídeo.
func (d *DB) ListVideoCategories(videoID int64) ([]string, error) {
	rows, err := d.db.Query(`SELECT c.path FROM categories c
		JOIN video_categories vt ON vt.category_id = c.id
		WHERE vt.video_id = ? ORDER BY c.path COLLATE NOCASE`, videoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		out = append(out, path)
	}
	return out, rows.Err()
}

// GetCategory devuelve el id de una categoría por ruta.
func (d *DB) GetCategory(path string) (int64, error) {
	var id int64
	err := d.db.QueryRow(`SELECT id FROM categories WHERE path = ? COLLATE NOCASE`, path).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// DeleteCategory borra una categoría; los vídeos quedan sin esa categoría.
func (d *DB) DeleteCategory(path string) error {
	res, err := d.db.Exec(`DELETE FROM categories WHERE path = ? COLLATE NOCASE`, path)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, path)
	}
	return nil
}

// ClearVideoPath quita la ruta del fichero local (vídeo borrado del disco,
// metadatos conservados).
func (d *DB) ClearVideoPath(id int64) error {
	_, err := d.db.Exec(`UPDATE videos SET path = NULL, updated_at = ? WHERE id = ?`,
		now(), id)
	return err
}

// RenameCategory renombra una categoría y sus subcategorías
// ("musica" → "jazz" actualiza "musica/80s" → "jazz/80s").
func (d *DB) RenameCategory(oldPath, newPath string) error {
	oldPath = strings.Trim(strings.TrimSpace(oldPath), "/")
	newPath = strings.Trim(strings.TrimSpace(newPath), "/")
	if oldPath == "" || newPath == "" {
		return fmt.Errorf("rutas no válidas")
	}
	if oldPath == SinCategoria || newPath == SinCategoria {
		return fmt.Errorf("%s es una categoría virtual", SinCategoria)
	}
	if strings.HasPrefix(newPath, oldPath+"/") || oldPath == newPath {
		return fmt.Errorf("ruta destino no válida")
	}
	// Renombrar la propia categoría si existe (si no, crearla en la nueva ruta).
	if _, err := d.db.Exec(
		`UPDATE categories SET path = ? WHERE path = ? COLLATE NOCASE`, newPath, oldPath); err != nil {
		return err
	}
	// Cascada sobre las subcategorías: substr conserva el "/" separador.
	_, err := d.db.Exec(
		`UPDATE categories SET path = ? || substr(path, ?)
		WHERE substr(path, 1, ?) = ? COLLATE NOCASE`,
		newPath, len(oldPath)+1, len(oldPath)+1, oldPath+"/")
	return err
}
