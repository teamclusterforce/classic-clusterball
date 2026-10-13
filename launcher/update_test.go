package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"v3.1", "3.0", true}, {"3.0", "3.0", false}, {"v3.0.1", "3.0", true}, {"3.0", "3.0.0", false},
		{"v2.9", "3.0", false}, {"v10.0", "9.9", true}, {"latest", "3.0", false}, {"v3.1-beta", "3.0", false},
	} {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestMsiAsset(t *testing.T) {
	r := &ghRelease{Assets: []ghAsset{{Name: "notes.txt"}, {Name: "Clusterball Re-Enhanced.MSI"}}}
	if a := msiAsset(r); a == nil || a.Name != "Clusterball Re-Enhanced.MSI" {
		t.Fatalf("msiAsset: %+v", a)
	}
	if msiAsset(&ghRelease{}) != nil {
		t.Error("ohne Installer muss nil kommen")
	}
}

// Der Download landet vollstaendig und geprueft im Zielordner; ein falscher
// SHA-256 laesst keine Datei zurueck.
func TestDownloadAsset(t *testing.T) {
	body := []byte("not really an msi, but bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	sum := sha256.Sum256(body)
	dir := t.TempDir()
	a := &ghAsset{Name: "Clusterball Re-Enhanced.msi", URL: srv.URL, Size: int64(len(body)),
		Digest: "sha256:" + hex.EncodeToString(sum[:])}
	var last int
	path, err := downloadAsset(context.Background(), a, dir, func(p int) { last = p })
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(body) || last != 100 {
		t.Errorf("Inhalt %q, Fortschritt %d", got, last)
	}
	a.Digest = "sha256:" + fmt.Sprintf("%064x", 0)
	os.Remove(path)
	if _, err := downloadAsset(context.Background(), a, dir, func(int) {}); err == nil {
		t.Error("falscher SHA-256 muss scheitern")
	}
	if files, _ := filepath.Glob(filepath.Join(dir, "*")); len(files) != 0 {
		t.Errorf("Reste: %v", files)
	}
}
