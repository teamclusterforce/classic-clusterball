package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lxn/win"
)

// Beide Voreinstellungen belegen jede Aktion, keine Taste doppelt, und jeder
// Name ist einer, den das Spiel kennt.
func TestVoreinstellungenVollstaendig(t *testing.T) {
	for name, p := range map[string]map[string]string{"klassisch": presetClassic, "wasd": presetWASD} {
		seen := map[string]string{}
		for _, act := range ctrlActions {
			n := p[act]
			if n == "" {
				t.Errorf("%s: %s nicht belegt", name, act)
				continue
			}
			if other, dup := seen[strings.ToLower(n)]; dup {
				t.Errorf("%s: %q doppelt (%s, %s)", name, n, other, act)
			}
			seen[strings.ToLower(n)] = act
			if _, ok := findGameKey(n); !ok && len(n) != 1 && n != wheelUpName && n != wheelDownName {
				t.Errorf("%s: %q kennt das Spiel nicht", name, n)
			}
		}
	}
}

// Die ausgelieferte config.cfg des Spiels (Ausschnitt) gilt als "klassisch"
// und wird beim ersten Start auf WASD umgestellt; eine eigene Belegung bleibt.
func TestMigrationWASD(t *testing.T) {
	shipped := map[string]string{
		"turn_left": "Linker Pfeil", "turn_right": "Rechter Pfeil", "pitch_up": "Pfeil runter", "pitch_down": "Pfeil Hoch",
		"roll": "e", "accelerate": "w", "brake": "s", "superboost": "Rechte Maustaste", "fire": "Linke Maustaste",
		"next_weapon": "d", "prev_weapon": "a", "change_target": "q", "release_balls": "Leertaste", "talk": "Enter",
		"toggle_scoreboard": "t", "change_view": "v",
	}
	cfg := AppConfig{Version: 2, Bindings: shipped}
	migrateConfig(&cfg)
	if cfg.Bindings["turn_left"] != "a" || cfg.Bindings["next_weapon"] != wheelUpName {
		t.Errorf("nicht auf WASD umgestellt: %v", cfg.Bindings)
	}

	own := map[string]string{}
	for k, v := range shipped {
		own[k] = v
	}
	own["fire"] = "x"
	cfg = AppConfig{Version: 2, Bindings: own}
	migrateConfig(&cfg)
	if cfg.Bindings["fire"] != "x" || cfg.Bindings["turn_left"] != "Linker Pfeil" {
		t.Errorf("eigene Belegung ueberschrieben: %v", cfg.Bindings)
	}
}

func TestSpracheDerTastennamen(t *testing.T) {
	if l := bindingsLang(map[string]string{"fire": "Linke Maustaste", "roll": "e"}); l != "de" {
		t.Errorf("deutsch erkannt als %q", l)
	}
	if l := bindingsLang(map[string]string{"fire": "Left mouse btn", "roll": "e"}); l != "en" {
		t.Errorf("englisch erkannt als %q", l)
	}
	en := presetIn(presetWASD, "en")
	if en["release_balls"] != "Space" || en["pitch_down"] != "Up arrow" || en["accelerate"] != "w" {
		t.Errorf("englische Voreinstellung: %v", en)
	}
}

func TestNameOfVK(t *testing.T) {
	for vk, want := range map[int]string{
		'W': "w", '7': "7", win.VK_F5: "F5", win.VK_F10: "F10", win.VK_F11: "", win.VK_F12: "",
		win.VK_LBUTTON: "Linke Maustaste", win.VK_XBUTTON1: "Maustaste 4", win.VK_UP: "Pfeil Hoch",
		win.VK_SPACE: "Leertaste", win.VK_LSHIFT: "Shift-Links", win.VK_RCONTROL: "STRG-Rechts",
	} {
		if got := nameOfVK(vk, "de"); got != want {
			t.Errorf("VK %#x: %q, erwartet %q", vk, got, want)
		}
	}
	if got := nameOfVK(win.VK_SPACE, "en"); got != "Space" {
		t.Errorf("VK_SPACE englisch: %q", got)
	}
}

// Schreiben und Lesen ueber die config.cfg: Latin-1-Namen bleiben Byte fuer
// Byte erhalten, alles andere in der Datei auch.
func TestBelegungInConfigCfg(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.cfg")
	orig := "game_volume 50.000000\nroll e\nbrake s\ntalkMessage4 Kein B\xe4lle?  :)\n \nfire Linke Maustaste\n"
	if err := os.WriteFile(path, []byte(orig), 0644); err != nil {
		t.Fatal(err)
	}
	g, err := LoadGameConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	b := readBindings(g)
	b["brake"] = "R\xfccktaste"
	b["fire"] = wheelUpName
	writeBindings(g, b)
	if err := g.Save(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	for _, want := range []string{"brake R\xfccktaste\n", "fire F11\n", "talkMessage4 Kein B\xe4lle?  :)\n", "\n \n", "roll e\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q fehlt in\n%q", want, s)
		}
	}
	if k, label, _ := capOf("R\xfccktaste"); k != capKey || label != "Rücktaste" {
		t.Errorf("Anzeige: %d %q", k, label)
	}
}

// Beim Start in der Sprache des Launchers stehen die Tastennamen der
// config.cfg in derselben Sprache; Buchstaben, F-Tasten und der Rest der
// Datei bleiben.
func TestBelegungInSpielsprache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.cfg")
	orig := "game_volume 50.000000\nroll e\nrelease_balls Leertaste\nfire Linke Maustaste\nnext_weapon F11\n"
	if err := os.WriteFile(path, []byte(orig), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &AppConfig{CBExePath: filepath.Join(dir, "Clusterball.exe"), Language: "en",
		Bindings: map[string]string{"release_balls": "Leertaste"}}
	if err := bindingsToGameLanguage(cfg); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	for _, want := range []string{"release_balls Space\n", "fire Left mouse btn\n", "roll e\n", "next_weapon F11\n", "game_volume 50.000000\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q fehlt in\n%q", want, s)
		}
	}
	if cfg.Bindings["release_balls"] != "Space" {
		t.Errorf("Launcher-Belegung: %q", cfg.Bindings["release_balls"])
	}
	cfg.Language = "de"
	if err := bindingsToGameLanguage(cfg); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "release_balls Leertaste\n") {
		t.Errorf("zurueck nach de:\n%q", data)
	}
}
