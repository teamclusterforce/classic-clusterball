package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ausschnitt aus einer echten config.cfg, mit genau den Eigenheiten, an denen
// ein naiver Schreiber scheitert: Werte mit Leerzeichen, ein leerer Wert, eine
// Zeile die nur aus einem Leerzeichen besteht, und ein abschliessender Umbruch.
// Achtung: "server_password " endet auf einem Leerzeichen und zwischen den
// Chat-Texten steht eine Zeile aus genau einem Leerzeichen - beides steht so
// in der echten Datei und darf beim Schreiben nicht verlorengehen.
const beispielCfg = "game_volume 50.000000\n" +
	"horiz_game_res 640.000000\n" +
	"vert_game_res 480.000000\n" +
	"antialias 0.000000\n" +
	"depth 16.000000\n" +
	"player_name Tester\n" +
	"team_name ohne Namen\n" +
	"server_password \n" +
	"turn_left Linker Pfeil\n" +
	"talkMessage1 Nimm das!!!\n" +
	" \n" +
	"graphicalQuality 2.000000\n" +
	"trilinearmipmap 0.000000\n"

func schreibeBeispiel(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.cfg")
	if err := os.WriteFile(path, []byte(beispielCfg), 0644); err != nil {
		t.Fatalf("Beispiel schreiben: %v", err)
	}
	return path
}

func TestGameConfigLesen(t *testing.T) {
	g, err := LoadGameConfig(schreibeBeispiel(t))
	if err != nil {
		t.Fatalf("laden: %v", err)
	}

	if got := g.Int("horiz_game_res", 0); got != 640 {
		t.Errorf("horiz_game_res = %d, erwartet 640", got)
	}
	if got := g.Int("graphicalQuality", -1); got != 2 {
		t.Errorf("graphicalQuality = %d, erwartet 2", got)
	}
	if g.Bool("antialias", true) {
		t.Error("antialias sollte aus sein")
	}
	// Werte mit Leerzeichen duerfen nicht am ersten Leerzeichen abgeschnitten werden.
	if v, _ := g.Get("team_name"); v != "ohne Namen" {
		t.Errorf("team_name = %q, erwartet \"ohne Namen\"", v)
	}
	if v, _ := g.Get("turn_left"); v != "Linker Pfeil" {
		t.Errorf("turn_left = %q", v)
	}
	if v, ok := g.Get("server_password"); !ok || v != "" {
		t.Errorf("server_password = %q, ok=%v", v, ok)
	}
	// Fehlender Schluessel liefert den Vorgabewert.
	if got := g.Int("gibtsnicht", 42); got != 42 {
		t.Errorf("Vorgabewert nicht verwendet: %d", got)
	}
}

func TestGameConfigSchreibenErhaeltRest(t *testing.T) {
	path := schreibeBeispiel(t)
	g, err := LoadGameConfig(path)
	if err != nil {
		t.Fatalf("laden: %v", err)
	}

	g.SetInt("horiz_game_res", 1024)
	g.SetInt("vert_game_res", 768)
	g.SetBool("antialias", true)
	g.Set("player_name", "Nigel")
	g.SetBool("force32BitTexture", true) // Schluessel fehlt noch -> anhaengen

	if err := g.Save(path); err != nil {
		t.Fatalf("speichern: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("zurueckleseen: %v", err)
	}
	out := string(data)

	// Geaenderte Werte
	for _, want := range []string{
		"horiz_game_res 1024.000000",
		"vert_game_res 768.000000",
		"antialias 1.000000",
		"player_name Nigel",
		"force32BitTexture 1.000000",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("fehlt in der Ausgabe: %q", want)
		}
	}

	// Unberuehrte Zeilen muessen exakt erhalten bleiben.
	for _, want := range []string{
		"game_volume 50.000000",
		"team_name ohne Namen",
		"server_password ",
		"turn_left Linker Pfeil",
		"talkMessage1 Nimm das!!!",
		"graphicalQuality 2.000000",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("verlorene Zeile: %q", want)
		}
	}

	// Kein Wert darf doppelt auftauchen.
	if n := strings.Count(out, "horiz_game_res"); n != 1 {
		t.Errorf("horiz_game_res steht %dx in der Datei", n)
	}
	// Die Zeile aus einem einzelnen Leerzeichen bleibt erhalten und der
	// neue Schluessel wird nicht dahinter ins Leere gehaengt.
	if !strings.Contains(out, "\n \n") {
		t.Error("die Leerzeichen-Zeile wurde verschluckt")
	}
	if strings.HasSuffix(out, "\n\n") {
		t.Error("zusaetzliche Leerzeile am Dateiende")
	}
}

func TestGameConfigZweimalSchreibenIstStabil(t *testing.T) {
	path := schreibeBeispiel(t)

	schreiben := func() string {
		g, err := LoadGameConfig(path)
		if err != nil {
			t.Fatalf("laden: %v", err)
		}
		g.SetInt("depth", 32)
		g.SetBool("trilinearmipmap", true)
		if err := g.Save(path); err != nil {
			t.Fatalf("speichern: %v", err)
		}
		data, _ := os.ReadFile(path)
		return string(data)
	}

	erst := schreiben()
	zweit := schreiben()
	if erst != zweit {
		t.Error("zweiter Schreibvorgang veraendert die Datei erneut")
	}
}

// readGameSettings muss jeden Grafikwert aus der Spieldatei uebernehmen -
// ein vergessener Schluessel zeigt in der Oberflaeche stillschweigend den
// Vorgabewert statt dem, was das Spiel tatsaechlich benutzt.
func TestReadGameSettingsUebernimmtAlles(t *testing.T) {
	dir := t.TempDir()
	inhalt := "depth 32.000000\n" +
		"graphicalQuality 1.000000\n" +
		"brightness 55.000000\n" +
		"antialias 1.000000\n" +
		"trilinearmipmap 1.000000\n" +
		"force32BitTexture 1.000000\n" +
		"clamp_texture 1.000000\n"
	if err := os.WriteFile(filepath.Join(dir, "config.cfg"), []byte(inhalt), 0644); err != nil {
		t.Fatalf("schreiben: %v", err)
	}

	// Vorgaben, die sich von der Datei unterscheiden - so faellt auf, wenn
	// ein Wert gar nicht erst gelesen wird.
	cfg := AppConfig{
		CBExePath:        filepath.Join(dir, "Clusterball.exe"),
		Depth:            16,
		GraphicalQuality: 2,
		Brightness:       90,
	}
	readGameSettings(&cfg)

	if cfg.Depth != 32 {
		t.Errorf("Depth = %d, erwartet 32", cfg.Depth)
	}
	if cfg.GraphicalQuality != 1 {
		t.Errorf("GraphicalQuality = %d, erwartet 1", cfg.GraphicalQuality)
	}
	if cfg.Brightness != 55 {
		t.Errorf("Brightness = %d, erwartet 55", cfg.Brightness)
	}
	if !cfg.Antialias {
		t.Error("Antialias nicht uebernommen")
	}
	if !cfg.TrilinearMipmap {
		t.Error("TrilinearMipmap nicht uebernommen")
	}
	if !cfg.Force32BitTex {
		t.Error("Force32BitTex nicht uebernommen")
	}
	if !cfg.ClampTexture {
		t.Error("ClampTexture nicht uebernommen")
	}
}

func TestGlideEnvSetztAlleVariablen(t *testing.T) {
	cfg := AppConfig{
		GlideResolution: "1920x1080",
		GlideAspect:     "4:3",
		GlideVSync:      true,
		GlideSplash:     false,
	}
	env := glideEnv(&cfg)

	will := map[string]string{
		"NGLIDE_RESOLUTION": "1920x1080",
		"NGLIDE_ASPECT":     "4:3",
		"NGLIDE_VSYNC":      "1",
		"NGLIDE_SPLASH":     "0",
	}
	for k, v := range will {
		gefunden := false
		for _, e := range env {
			if e == k+"="+v {
				gefunden = true
				break
			}
		}
		if !gefunden {
			t.Errorf("%s=%s fehlt in der Umgebung", k, v)
		}
	}
	// Die geerbte Umgebung muss erhalten bleiben, sonst startet das Spiel
	// ohne PATH und SystemRoot.
	if len(env) <= len(will) {
		t.Errorf("Umgebung enthaelt nur %d Eintraege - geerbte fehlen", len(env))
	}
}
