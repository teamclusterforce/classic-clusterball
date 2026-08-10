package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GameConfig ist die config.cfg des Spiels: Zeilen der Form "schluessel wert".
//
// Die Datei enthält auch Tastenbelegungen, Chat-Texte und Flugmodell-Werte,
// teils mit Leerzeichen im Wert und einer Zeile, die nur aus einem Leerzeichen
// besteht. Deshalb werden die Originalzeilen behalten und beim Schreiben nur
// die Zeilen ersetzt, die wir tatsächlich ändern - alles andere bleibt Byte
// für Byte stehen.
type GameConfig struct {
	lines []string
	index map[string]int
}

// gameConfigPath liefert den Pfad der config.cfg neben der Clusterball.exe.
func gameConfigPath(cfg *AppConfig) string {
	dir := cbExeDir(cfg)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "config.cfg")
}

func LoadGameConfig(path string) (*GameConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	g := &GameConfig{
		lines: strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"),
		index: make(map[string]int),
	}
	for i, line := range g.lines {
		if k, _, ok := splitEntry(line); ok {
			if _, dup := g.index[k]; !dup {
				g.index[k] = i
			}
		}
	}
	return g, nil
}

// splitEntry trennt "schluessel wert" am ersten Leerzeichen.
func splitEntry(line string) (key, value string, ok bool) {
	if strings.TrimSpace(line) == "" {
		return "", "", false
	}
	k, v, found := strings.Cut(line, " ")
	if !found {
		return k, "", true
	}
	return k, v, true
}

func (g *GameConfig) Get(key string) (string, bool) {
	i, ok := g.index[key]
	if !ok {
		return "", false
	}
	_, v, _ := splitEntry(g.lines[i])
	return v, true
}

// Num liest einen Zahlenwert. Das Spiel schreibt alles als Gleitkomma
// ("1024.000000"), auch die Schalter.
func (g *GameConfig) Num(key string, def float64) float64 {
	v, ok := g.Get(key)
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return def
	}
	return f
}

func (g *GameConfig) Int(key string, def int) int {
	return int(g.Num(key, float64(def)))
}

func (g *GameConfig) Bool(key string, def bool) bool {
	d := 0.0
	if def {
		d = 1.0
	}
	return g.Num(key, d) != 0
}

func (g *GameConfig) Set(key, value string) {
	line := key + " " + value
	if i, ok := g.index[key]; ok {
		g.lines[i] = line
		return
	}
	// Am Dateiende haengt durch den abschliessenden Zeilenumbruch ein leerer
	// Eintrag - dort einfuegen statt dahinter, sonst entsteht eine Leerzeile.
	if n := len(g.lines); n > 0 && strings.TrimSpace(g.lines[n-1]) == "" {
		g.lines[n-1] = line
		g.lines = append(g.lines, "")
		g.index[key] = n - 1
		return
	}
	g.lines = append(g.lines, line)
	g.index[key] = len(g.lines) - 1
}

func (g *GameConfig) SetNum(key string, v float64) { g.Set(key, fmt.Sprintf("%f", v)) }
func (g *GameConfig) SetInt(key string, v int)     { g.SetNum(key, float64(v)) }

func (g *GameConfig) SetBool(key string, b bool) {
	if b {
		g.SetNum(key, 1)
		return
	}
	g.SetNum(key, 0)
}

func (g *GameConfig) Save(path string) error {
	return os.WriteFile(path, []byte(strings.Join(g.lines, "\n")), 0644)
}

// ── Abgleich mit der Launcher-Konfiguration ──────────────────────────────────

// readGameSettings uebernimmt die Grafikwerte aus der config.cfg des Spiels.
// Die Spieldatei ist dafuer die maßgebliche Quelle: sonst zeigt der Launcher
// Werte an, die das Spiel nie gesehen hat - genau das war vorher der Fall, die
// eingestellte Auflösung wurde nie übertragen.
func readGameSettings(cfg *AppConfig) {
	path := gameConfigPath(cfg)
	if path == "" {
		return
	}
	g, err := LoadGameConfig(path)
	if err != nil {
		return
	}
	// Auflösung bewusst nicht uebernehmen: der Launcher setzt immer das
	// Maximum des Spiels, ein niedrigerer Wert in der Datei ist genau das,
	// was hier korrigiert werden soll.
	cfg.Depth = g.Int("depth", cfg.Depth)
	cfg.GraphicalQuality = g.Int("graphicalQuality", cfg.GraphicalQuality)
	cfg.Brightness = g.Int("brightness", cfg.Brightness)
	cfg.Antialias = g.Bool("antialias", cfg.Antialias)
	cfg.TrilinearMipmap = g.Bool("trilinearmipmap", cfg.TrilinearMipmap)
	cfg.Force32BitTex = g.Bool("force32BitTexture", cfg.Force32BitTex)
	cfg.ClampTexture = g.Bool("clamp_texture", cfg.ClampTexture)
}

// writeGameSettings schreibt Zugangsdaten und Grafikwerte in die config.cfg.
// Der Renderer-Eintrag bleibt bewusst unangetastet: config.cfg zaehlt anders
// als XDreamCC.ini, und ein falscher Index waere ein nicht startendes Spiel.
func writeGameSettings(cfg *AppConfig) error {
	path := gameConfigPath(cfg)
	if path == "" {
		return nil
	}
	g, err := LoadGameConfig(path)
	if err != nil {
		return err
	}

	if cfg.AccountName != "" {
		g.Set("player_name", cfg.AccountName)
		g.Set("player_password", cfg.AccountPassword)
	}

	g.SetInt("horiz_game_res", cfg.Width)
	g.SetInt("vert_game_res", cfg.Height)
	g.SetInt("depth", cfg.Depth)
	g.SetInt("graphicalQuality", cfg.GraphicalQuality)
	g.SetInt("brightness", cfg.Brightness)
	g.SetBool("antialias", cfg.Antialias)
	g.SetBool("trilinearmipmap", cfg.TrilinearMipmap)
	g.SetBool("force32BitTexture", cfg.Force32BitTex)
	g.SetBool("clamp_texture", cfg.ClampTexture)

	return g.Save(path)
}

// ── nGlide ───────────────────────────────────────────────────────────────────

// Das mitgelieferte glide2x.dll ist nicht der 3dfx-Originaltreiber, sondern
// nGlide 2.1 (Zeus Software) mit übernommener 3dfx-Versionsressource - daher
// auch das 3dfx-Logo beim Start. nGlide liest seine Einstellungen in dieser
// Reihenfolge: Umgebungsvariable, dann Registry, dann Vorgabe.
//
// Der Launcher setzt sie deshalb als Umgebungsvariablen nur fuer den
// gestarteten Spielprozess: nichts wird global veraendert, andere Glide-Spiele
// bleiben unberührt, und ein Zuruecknehmen erfordert kein Aufraeumen.
func glideEnv(cfg *AppConfig) []string {
	env := os.Environ()
	add := func(k, v string) { env = append(env, k+"="+v) }

	add("NGLIDE_RESOLUTION", cfg.GlideResolution)
	add("NGLIDE_ASPECT", cfg.GlideAspect)
	add("NGLIDE_VSYNC", boolStr(cfg.GlideVSync))
	add("NGLIDE_SPLASH", boolStr(cfg.GlideSplash))
	return env
}
