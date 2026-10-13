package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Eine alte xdreamcc.cfg wird gelesen, solange es keine clusterball.ini
// gibt; nach dem Speichern steht alles in clusterball.ini und die alte
// Datei ist weg.
func TestAlteKonfigWirdClusterballIni(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, legacyConfigName)
	if err := os.WriteFile(old, []byte("language=en\r\naccount_name=pilot\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := configToRead(dir); got != old {
		t.Fatalf("liest %q, erwartet die alte Datei", got)
	}
	cfg := defaultConfig
	readAppConfigFrom(&cfg, configToRead(dir))
	if cfg.Language != "en" || cfg.AccountName != "pilot" {
		t.Fatalf("aus der alten Datei: language=%q account=%q", cfg.Language, cfg.AccountName)
	}
	if err := saveConfigIn(&cfg, dir); err != nil {
		t.Fatal(err)
	}
	if fileExists(old) {
		t.Error("xdreamcc.cfg nach dem Speichern noch vorhanden")
	}
	if got := configToRead(dir); got != filepath.Join(dir, configName) {
		t.Fatalf("liest danach %q", got)
	}
	back := defaultConfig
	readAppConfigFrom(&back, configToRead(dir))
	if back.Language != "en" || back.AccountName != "pilot" {
		t.Errorf("aus clusterball.ini: language=%q account=%q", back.Language, back.AccountName)
	}
}

func TestMigrationRespektiertBewussteWahl(t *testing.T) {
	// Wer nach der Migration bewusst zurückstellt, hat die aktuelle Version
	// gespeichert - das darf nicht erneut überschrieben werden.
	cfg := AppConfig{
		GraphicalQuality: 0,
		Antialias:        false,
		Version:          configVersion,
	}
	migrateConfig(&cfg)

	if cfg.GraphicalQuality != 0 || cfg.Antialias {
		t.Error("Bildqualität überschrieben, obwohl bewusst gesetzt")
	}
}

// Version 2 hebt die Bildqualität einmalig auf die Empfehlung an, auch wenn
// in der config.cfg des Spiels etwas anderes stand.
func TestMigrationSetztBildqualitaet(t *testing.T) {
	cfg := AppConfig{
		Version:          1,
		GraphicalQuality: 0,
		Brightness:       40,
		Antialias:        false,
		TrilinearMipmap:  false,
		Force32BitTex:    false,
		ClampTexture:     false,
	}
	migrateConfig(&cfg)

	if cfg.GraphicalQuality != gameQuality {
		t.Errorf("GraphicalQuality = %d, erwartet %d (hoch)", cfg.GraphicalQuality, gameQuality)
	}
	if cfg.Brightness != 90 {
		t.Errorf("Brightness = %d, erwartet 90", cfg.Brightness)
	}
	if !cfg.Antialias || !cfg.TrilinearMipmap || !cfg.Force32BitTex || !cfg.ClampTexture {
		t.Errorf("Qualitätsschalter nicht gesetzt: %+v", cfg)
	}
	if cfg.Version != configVersion {
		t.Errorf("Version = %d, erwartet %d", cfg.Version, configVersion)
	}
}

func TestVorgaben(t *testing.T) {
	if defaultConfig.Depth != gameDepth {
		t.Errorf("Farbtiefe = %d, erwartet %d", defaultConfig.Depth, gameDepth)
	}
	if !defaultConfig.Antialias || !defaultConfig.TrilinearMipmap ||
		!defaultConfig.Force32BitTex || !defaultConfig.ClampTexture {
		t.Errorf("Qualitätsschalter nicht vorbelegt: %+v", defaultConfig)
	}
	if defaultConfig.GraphicalQuality != gameQuality || defaultConfig.Brightness != 90 {
		t.Errorf("Detailstufe/Helligkeit falsch: %d / %d",
			defaultConfig.GraphicalQuality, defaultConfig.Brightness)
	}
	// Direct3D: Vollbild in nativer Auflösung, scharfe Schrift an.
	d := defaultConfig.D3D
	if !d.Fullscreen || d.Resolution != "" || !d.FontReplace {
		t.Errorf("Direct3D-Vorgaben falsch: %+v", d)
	}
}

// Unter der Direct3D-Schicht startet das Spiel nur mit 32 Bit.
func TestFarbtiefe32(t *testing.T) {
	if gameDepth != 32 {
		t.Errorf("gameDepth = %d, erwartet 32", gameDepth)
	}
}

// Jede Auswahlliste braucht zu jedem Wert eine Beschriftung.
func TestAuswahllistenVollstaendig(t *testing.T) {
	pairs := []struct {
		name   string
		values int
		labels int
	}{
		{"Anzeige", len(displayValues), len(displayLabels())},
		{"Auflösung", len(resValues), len(resLabels(resValues))},
		{"Kantenglättung", len(msaaValues), len(msaaLabels())},
		{"Supersampling", len(ssValues), len(ssLabels())},
		{"Filterung", len(anisoValues), len(anisoLabels())},
		{"Texturen", len(packValues), len(packLabels())},
	}
	for _, p := range pairs {
		if p.values != p.labels {
			t.Errorf("%s: %d Werte, %d Beschriftungen", p.name, p.values, p.labels)
		}
	}
}
