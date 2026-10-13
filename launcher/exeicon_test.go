package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// Das Gruppenverzeichnis zeigt der Reihe nach auf die vergebenen IDs, und
// jedes Bild ist genau so lang, wie sein Eintrag sagt.
func TestIconGroup(t *testing.T) {
	g, images, err := iconGroup(appIconICO, 10)
	if err != nil {
		t.Fatal(err)
	}
	n := int(binary.LittleEndian.Uint16(g[4:]))
	if n != len(images) || len(g) != 6+14*n {
		t.Fatalf("%d Einträge, %d Bilder, %d Bytes", n, len(images), len(g))
	}
	for i := 0; i < n; i++ {
		e := g[6+14*i:]
		if id := binary.LittleEndian.Uint16(e[12:]); id != uint16(10+i) {
			t.Errorf("Eintrag %d: ID %d", i, id)
		}
		if size := binary.LittleEndian.Uint32(e[8:]); int(size) != len(images[i]) {
			t.Errorf("Eintrag %d: %d Bytes angegeben, Bild hat %d", i, size, len(images[i]))
		}
	}
}

// Mit CB_TEST_EXE=<Pfad zu Clusterball.exe>: eine Kopie bekommt das Icon,
// ein zweiter Aufruf ändert nichts mehr.
func TestGameIconOnCopy(t *testing.T) {
	src := os.Getenv("CB_TEST_EXE")
	if src == "" {
		t.Skip("CB_TEST_EXE nicht gesetzt")
	}
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "Clusterball.exe")
	if err := os.WriteFile(exe, b, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureGameIcon(exe); err != nil {
		t.Fatal(err)
	}
	if ok, err := gameIconInstalled(exe); err != nil || !ok {
		t.Fatalf("Icon nicht installiert (%v)", err)
	}
	if err := checkPatchedExe(b, exe); err != nil {
		t.Fatal(err)
	}
	st1, _ := os.Stat(exe)
	if err := ensureGameIcon(exe); err != nil {
		t.Fatal(err)
	}
	st2, _ := os.Stat(exe)
	if !st1.ModTime().Equal(st2.ModTime()) {
		t.Error("zweiter Aufruf hat die exe erneut geschrieben")
	}
	if out := os.Getenv("CB_TEST_EXE_OUT"); out != "" {
		nb, _ := os.ReadFile(exe)
		os.WriteFile(out, nb, 0o755)
	}
}
