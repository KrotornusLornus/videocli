package download

import "testing"

func TestParseOutput(t *testing.T) {
	// Import (--print to_json): solo JSON.
	metaData, filePath, err := parseOutput([]string{
		`{"id":"abc","title":"Mi vídeo","duration":12.5,"webpage_url":"https://x/abc"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if filePath != "" || metaData.Title != "Mi vídeo" || metaData.Duration != 12.5 {
		t.Fatalf("import: %+v %q", metaData, filePath)
	}

	// Fetch (to_json + after_move:filepath).
	metaData, filePath, err = parseOutput([]string{
		`{"title":"Otro","duration":3}`,
		"/home/u/Videos/videocli/Otro [abc].mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if metaData.Title != "Otro" || filePath != "/home/u/Videos/videocli/Otro [abc].mp4" {
		t.Fatalf("fetch: %+v %q", metaData, filePath)
	}

	// Líneas ruidosas se ignoran; JSON vacío no es error.
	metaData, filePath, err = parseOutput([]string{"", "[youtube] descargando…", `{}`})
	if err != nil || metaData == nil || filePath != "" {
		t.Fatalf("ruido: %+v %q %v", metaData, filePath, err)
	}
}
