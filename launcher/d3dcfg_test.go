package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Die mitgelieferte Vorlage muss die Vorgaben der Schicht ergeben.
func TestVorlageErgibtVorgaben(t *testing.T) {
	dir := t.TempDir()
	cfg := AppConfig{CBExePath: filepath.Join(dir, "Clusterball.exe")}
	readD3DSettings(&cfg) // keine INI im Ordner -> Vorlage
	if cfg.D3D != defaultD3D {
		t.Errorf("Vorlage = %+v\nerwartet %+v", cfg.D3D, defaultD3D)
	}
}

// Das 16-fach-Paket wird nicht mehr ausgeliefert: eine ältere INI, die es
// noch wählt, bekommt das 4-fach-Paket.
func TestHD16WirdHD(t *testing.T) {
	dir := t.TempDir()
	inhalt := "[d3d]\r\nTextureReplace=1\r\nTexturePack=hd16\r\n"
	if err := os.WriteFile(filepath.Join(dir, d3dIniName), []byte(inhalt), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := AppConfig{CBExePath: filepath.Join(dir, "Clusterball.exe")}
	readD3DSettings(&cfg)
	if cfg.D3D.TexturePack != "hd" {
		t.Errorf("TexturePack = %q, erwartet \"hd\"", cfg.D3D.TexturePack)
	}
}

// Schreiben ändert nur die verwalteten Schlüssel; Werte, die von Hand
// eingetragen wurden, bleiben stehen - auch solche, die die Vorlage nicht
// kennt. Die Kommentare kommen aus der aktuellen Vorlage.
func TestD3DSchreibenErhaeltRest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, d3dIniName)
	inhalt := "; alter Kopf\r\n[d3d]\r\nResolution=\r\nContrast=1.2\r\nMSAA=4\r\nEigenerSchluessel=7\r\n"
	if err := os.WriteFile(path, []byte(inhalt), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := AppConfig{CBExePath: filepath.Join(dir, "Clusterball.exe"), D3D: defaultD3D}
	cfg.D3D.Resolution = "1920x1080"
	cfg.D3D.Fullscreen = false
	cfg.D3D.TexturePack = ""
	cfg.D3D.Sharpen = 25
	if err := writeD3DSettings(&cfg); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	out := string(data)
	for _, want := range []string{"Clusterball Re-Enhanced", "[d3d]", "Contrast=1.2", "EigenerSchluessel=7",
		"Resolution=1920x1080", "Fullscreen=0", "TextureReplace=0", "Sharpen=0.25"} {
		if !strings.Contains(out, want) {
			t.Errorf("fehlt: %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "alter Kopf") {
		t.Error("veralteter Kommentar aus der alten Datei übernommen")
	}
	if strings.Count(out, "MSAA=") != 1 {
		t.Error("MSAA steht doppelt in der Datei")
	}

	// Zurücklesen ergibt dieselben Werte.
	back := AppConfig{CBExePath: cfg.CBExePath}
	readD3DSettings(&back)
	if back.D3D != cfg.D3D {
		t.Errorf("zurückgelesen %+v\nerwartet %+v", back.D3D, cfg.D3D)
	}
}

func TestNormalizeResolution(t *testing.T) {
	for in, want := range map[string]string{
		"":            "",
		"native":      "",
		"1920x1080":   "1920x1080",
		"1920 × 1080": "1920x1080",
		"2560X1440":   "2560x1440",
		"100x100":     "",
	} {
		if got := normalizeResolution(in); got != want {
			t.Errorf("normalizeResolution(%q) = %q, erwartet %q", in, got, want)
		}
	}
}

// installLayer legt die Schicht ab und entfernt die alten Glide-Dateien.
func TestInstallLayer(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"opengl32.dll", "glide2x.dll", "3DfxSpl2.dll"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("alt"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := installLayer(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "opengl32.dll"))
	if err != nil || !bytes.Equal(data, layerDLL) {
		t.Error("opengl32.dll ist nicht die mitgelieferte Schicht")
	}
	for _, name := range legacyRenderFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s liegt noch im Ordner", name)
		}
	}
	// Ein zweiter Aufruf ändert nichts und scheitert nicht.
	if err := installLayer(dir); err != nil {
		t.Fatal(err)
	}
}

// Der Schrift-Ersatz hat das Format, das die Schicht erwartet: 1024 × 2048,
// doppelt hohe Zellen (font_tall). Ohne Bahnschrift entfällt der Test.
func TestFontAtlas(t *testing.T) {
	if _, err := os.Stat(systemFontPath("bahnschrift.ttf")); err != nil {
		t.Skip("Bahnschrift nicht installiert")
	}
	dir := t.TempDir()
	if err := ensureFontAtlas(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(fontAtlasPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:4]) != "DDS " {
		t.Fatal("keine DDS-Datei")
	}
	h, w := binary.LittleEndian.Uint32(data[12:]), binary.LittleEndian.Uint32(data[16:])
	if w != 1024 || h != 2048 {
		t.Errorf("Größe %dx%d, erwartet 1024x2048", w, h)
	}
	// Die Zelle von "A" (Zeile 2, Spalte 1) muss Deckung haben.
	cov := 0
	for y := 2*fontCellH + fontYOff; y < 2*fontCellH+fontYOff+fontCellW; y++ {
		for x := fontCellW; x < 2*fontCellW; x++ {
			if data[128+(y*1024+x)*4+3] > 128 {
				cov++
			}
		}
	}
	if cov < 300 {
		t.Errorf("Zelle 'A' fast leer: %d deckende Pixel", cov)
	}
}
