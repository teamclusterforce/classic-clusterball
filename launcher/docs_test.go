package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func normLF(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// Die eingebetteten Kopien entsprechen LICENSE und NOTICE im Repo-Stamm.
func TestDocsSindAktuell(t *testing.T) {
	for emb, root := range map[string]string{"docs/License.txt": "../LICENSE", "docs/Notice.txt": "../NOTICE"} {
		a, err := reDocs.ReadFile(emb)
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(root)
		if err != nil {
			t.Fatal(err)
		}
		if normLF(string(a)) != normLF(string(b)) {
			t.Errorf("%s weicht von %s ab - neu kopieren", emb, root)
		}
	}
}

func TestInstallDocs(t *testing.T) {
	dir := t.TempDir()
	if err := installDocs(dir); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"Readme.txt", "License.txt", "Notice.txt"} {
		d, err := os.ReadFile(filepath.Join(dir, docsPrefix+n))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(d), "\r\n") || strings.Contains(strings.ReplaceAll(string(d), "\r\n", ""), "\n") {
			t.Errorf("%s: keine Windows-Zeilenenden", n)
		}
	}
}
