// Package db gestiona la base de datos SQLite de la videoteca.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB envuelve la base de datos.
type DB struct {
	db *sql.DB
}

// ErrNotFound se devuelve cuando no hay registro para una clave dada.
var ErrNotFound = fmt.Errorf("no encontrado")

const schemaV1 = `
CREATE TABLE IF NOT EXISTS videos (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	title        TEXT NOT NULL DEFAULT '',
	url          TEXT,
	path         TEXT NOT NULL UNIQUE,
	thumb_path   TEXT NOT NULL DEFAULT '',
	content_hash TEXT NOT NULL DEFAULT '',
	hash_size    INTEGER NOT NULL DEFAULT 0,
	duration_ms  INTEGER NOT NULL DEFAULT 0,
	width        INTEGER NOT NULL DEFAULT 0,
	height       INTEGER NOT NULL DEFAULT 0,
	video_codec  TEXT NOT NULL DEFAULT '',
	audio_codec  TEXT NOT NULL DEFAULT '',
	status       TEXT NOT NULL DEFAULT 'ok',
	error        TEXT NOT NULL DEFAULT '',
	source       TEXT NOT NULL DEFAULT 'local',
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_videos_hash  ON videos(content_hash, hash_size);
CREATE INDEX IF NOT EXISTS idx_videos_thumb ON videos(thumb_path);

CREATE TABLE IF NOT EXISTS tags (
	id   INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE COLLATE NOCASE
);

CREATE TABLE IF NOT EXISTS video_tags (
	video_id INTEGER NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
	tag_id   INTEGER NOT NULL REFERENCES tags(id)   ON DELETE CASCADE,
	PRIMARY KEY (video_id, tag_id)
);

CREATE TABLE IF NOT EXISTS downloads (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	url         TEXT NOT NULL,
	video_id    INTEGER REFERENCES videos(id) ON DELETE SET NULL,
	status      TEXT NOT NULL DEFAULT 'pending',
	error       TEXT NOT NULL DEFAULT '',
	created_at  TEXT NOT NULL,
	finished_at TEXT
);
`

// schemaV2 permite path NULL (importaciones de solo metadatos, sin vídeo
// local); el resto de tablas no cambia.
var schemaV2 = strings.Replace(schemaV1, "path         TEXT NOT NULL UNIQUE,",
	"path         TEXT UNIQUE,", 1)

// Open abre (y si hace falta crea) la base de datos en path.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	if err := migrate(sqlDB); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return &DB{db: sqlDB}, nil
}

func migrate(sqlDB *sql.DB) error {
	var version int
	if err := sqlDB.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version >= 3 {
		return nil
	}
	// Las migraciones que recrean tablas necesitan desactivar las claves
	// foráneas temporalmente (no puede estar dentro de la transacción).
	if _, err := sqlDB.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	defer sqlDB.Exec("PRAGMA foreign_keys = ON")
	tx, err := sqlDB.Begin()
	if err != nil {
		return err
	}
	rollback := func(e error) error {
		tx.Rollback()
		return e
	}
	if version < 1 {
		if _, err := tx.Exec(schemaV3); err != nil {
			return rollback(err)
		}
	} else {
		if version < 2 {
			// v1 → v2: recrear videos con path NULL-able, copiando datos.
			// (SQLite no puede quitar NOT NULL con ALTER TABLE.)
			mig := strings.Replace(schemaV2, "CREATE TABLE IF NOT EXISTS videos",
				"CREATE TABLE videos_mig", 1)
			mig = strings.Replace(mig, "CREATE TABLE IF NOT EXISTS videos_mig (",
				"CREATE TABLE videos_mig (", 1)
			if _, err := tx.Exec(strings.Replace(mig,
				"CREATE TABLE videos_mig", "CREATE TABLE videos_mig", 1)); err != nil {
				return rollback(err)
			}
			cols := "id, title, url, path, thumb_path, content_hash, hash_size,\nduration_ms, width, height, video_codec, audio_codec, status, error, source,\ncreated_at, updated_at"
			if _, err := tx.Exec(`INSERT INTO videos_mig (` + cols + `)
				SELECT ` + cols + ` FROM videos`); err != nil {
				return rollback(err)
			}
			if _, err := tx.Exec(`DROP TABLE videos`); err != nil {
				return rollback(err)
			}
			if _, err := tx.Exec(`ALTER TABLE videos_mig RENAME TO videos`); err != nil {
				return rollback(err)
			}
			if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_videos_hash ON videos(content_hash, hash_size);
				CREATE INDEX IF NOT EXISTS idx_videos_thumb ON videos(thumb_path)`); err != nil {
				return rollback(err)
			}
		}
		if version < 3 {
			// v2 → v3: fuera etiquetas, dentro categorías-ruta.
			if _, err := tx.Exec(`DROP TABLE IF EXISTS video_tags;
				DROP TABLE IF EXISTS tags;
				CREATE TABLE IF NOT EXISTS categories (
					id   INTEGER PRIMARY KEY AUTOINCREMENT,
					path TEXT NOT NULL UNIQUE COLLATE NOCASE
				);
				CREATE TABLE IF NOT EXISTS video_categories (
					video_id     INTEGER NOT NULL REFERENCES videos(id)     ON DELETE CASCADE,
					category_id  INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
					PRIMARY KEY (video_id, category_id)
				)`); err != nil {
				return rollback(err)
			}
		}
	}
	if _, err := tx.Exec("PRAGMA user_version = 3"); err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

// Close cierra la base de datos.
func (d *DB) Close() error { return d.db.Close() }

// now devuelve el instante actual en RFC3339 UTC.
func now() string { return time.Now().UTC().Format(time.RFC3339) }

// schemaV3 sustituye las etiquetas por categorías-ruta (estilo directorio).
// Sin-categoria es virtual: solo son vídeos sin ninguna categoría.
var schemaV3 = strings.Replace(schemaV2,
	`CREATE TABLE IF NOT EXISTS tags (
	id   INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE COLLATE NOCASE
);

CREATE TABLE IF NOT EXISTS video_tags (
	video_id INTEGER NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
	tag_id   INTEGER NOT NULL REFERENCES tags(id)   ON DELETE CASCADE,
	PRIMARY KEY (video_id, tag_id)
);`,
	`CREATE TABLE IF NOT EXISTS categories (
	id   INTEGER PRIMARY KEY AUTOINCREMENT,
	path TEXT NOT NULL UNIQUE COLLATE NOCASE
);

CREATE TABLE IF NOT EXISTS video_categories (
	video_id     INTEGER NOT NULL REFERENCES videos(id)     ON DELETE CASCADE,
	category_id  INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
	PRIMARY KEY (video_id, category_id)
);`, 1)
