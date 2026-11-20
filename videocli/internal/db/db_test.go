package db

import (
	"path/filepath"
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	database, err := Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestCategoriesCRUD(t *testing.T) {
	database := testDB(t)
	id, err := database.EnsureCategory("musica/80s")
	if err != nil {
		t.Fatal(err)
	}
	// Idempotente, insensible a mayúsculas y normaliza barras sobrantes.
	id2, err := database.EnsureCategory(" musica/80s/ ")
	if err != nil {
		t.Fatal(err)
	}
	if id != id2 {
		t.Fatalf("EnsureCategory debería reutilizar: %d != %d", id, id2)
	}
	// La categoría virtual no debe poder crearse.
	if _, err := database.EnsureCategory("sin-categoria"); err == nil {
		t.Fatal("sin-categoria no debe ser una categoría real")
	}

	v := &Video{Path: "/tmp/v.mp4"}
	if _, err := database.InsertVideo(v); err != nil {
		t.Fatal(err)
	}
	if err := database.AssignCategory(v.ID, id); err != nil {
		t.Fatal(err)
	}
	cats, err := database.ListVideoCategories(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) != 1 || cats[0] != "musica/80s" {
		t.Fatalf("categorías incorrectas: %v", cats)
	}

	all, err := database.ListCategories()
	if err != nil || len(all) != 1 || all[0].Count != 1 {
		t.Fatalf("ListCategories: %+v err=%v", all, err)
	}

	if err := database.UnassignCategory(v.ID, id); err != nil {
		t.Fatal(err)
	}
	if err := database.DeleteCategory("MUSICA/80S"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetCategory("musica/80s"); err != ErrNotFound {
		t.Fatalf("la categoría debería desaparecer: %v", err)
	}
}

func TestFindByHashAndPath(t *testing.T) {
	database := testDB(t)
	v := &Video{Path: "/tmp/a.mp4", ContentHash: "abc", HashSize: 10}
	if _, err := database.InsertVideo(v); err != nil {
		t.Fatal(err)
	}
	if got, err := database.FindByHash("abc", 10); err != nil || got.ID != v.ID {
		t.Fatalf("FindByHash: %+v %v", got, err)
	}
	if _, err := database.FindByHash("abc", 11); err != ErrNotFound {
		t.Fatalf("tamaño distinto no debería coincidir: %v", err)
	}
	// Miniatura → vídeo.
	if err := database.UpdateVideoThumb(v.ID, "/tmp/thumbs/1.jpg"); err != nil {
		t.Fatal(err)
	}
	if got, err := database.FindByAnyPath("/tmp/thumbs/1.jpg"); err != nil || got.ID != v.ID {
		t.Fatalf("FindByAnyPath con miniatura: %+v %v", got, err)
	}
}

func TestListFilters(t *testing.T) {
	database := testDB(t)
	a := &Video{Path: "/tmp/a.mp4", ThumbPath: "/t/a.jpg"}
	b := &Video{Path: "/tmp/b.mp4", ThumbPath: "/t/b.jpg"}
	c := &Video{Path: "/tmp/c.mp4", ThumbPath: "/t/c.jpg"}
	for _, v := range []*Video{a, b, c} {
		if _, err := database.InsertVideo(v); err != nil {
			t.Fatal(err)
		}
	}
	musicaID, _ := database.EnsureCategory("musica")
	popID, _ := database.EnsureCategory("musica/pop")
	database.AssignCategory(a.ID, musicaID)
	database.AssignCategory(b.ID, popID)

	// Herencia: música incluye música/pop.
	byCat, err := database.ListVideos("MUSICA", "")
	if err != nil || len(byCat) != 2 {
		t.Fatalf("herencia: %d %v", len(byCat), err)
	}
	byPop, err := database.ListVideos("musica/pop", "")
	if err != nil || len(byPop) != 1 || byPop[0].ID != b.ID {
		t.Fatalf("exacta: %+v %v", byPop, err)
	}
	// sin-categoria: solo vídeos sin ninguna categoría.
	untagged, err := database.ListVideos(SinCategoria, "")
	if err != nil || len(untagged) != 1 || untagged[0].ID != c.ID {
		t.Fatalf("sin-categoria: %+v %v", untagged, err)
	}
	// Todos.
	all, err := database.ListVideos("", "")
	if err != nil || len(all) != 3 {
		t.Fatalf("all: %d %v", len(all), err)
	}

	// Borrar categoría no borra vídeos.
	if err := database.DeleteCategory("musica/pop"); err != nil {
		t.Fatal(err)
	}
	all2, err := database.ListVideos("", "")
	if err != nil || len(all2) != 3 {
		t.Fatalf("tras borrar categoría: %d %v", len(all2), err)
	}
	// b quedó sin categorías → sin-categoria ahora.
	untagged2, err := database.ListVideos(SinCategoria, "")
	if err != nil || len(untagged2) != 2 {
		t.Fatalf("sin-categoria tras borrar: %d %v", len(untagged2), err)
	}
}
