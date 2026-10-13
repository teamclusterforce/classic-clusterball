package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// D3DSettings sind die Einstellungen der Direct3D-Schicht (opengl32.dll neben
// der Clusterball.exe). Die Schicht liest sie aus clusterball_d3d.ini; die
// Datei ist damit die maßgebliche Quelle, genau wie config.cfg für die
// Einstellungen des Spiels selbst.
type D3DSettings struct {
	Fullscreen  bool    // randloses Vollbild über den ganzen Monitor, sonst Fenster
	Resolution  string  // Renderauflösung "BxH", leer = native Auflösung des Monitors
	MSAA        int     // Kantenglättung: 1 (aus), 2, 4, 8
	SuperSample int     // 1 (aus) oder 2 = in doppelter Auflösung rendern
	Anisotropy  int     // anisotrope Filterung: 1 (aus) … 16
	VSync       bool    // auf den Bildaufbau des Monitors warten
	Sharpen     int     // Nachschärfen 0 … 100 (in der INI 0 … 1)
	TexturePack string  // "" = Originaltexturen, "hd" = 4-fach
	FontReplace bool    // Menü- und HUD-Schrift aus der Bahnschrift statt 8 × 8 Pixel
	AmbientOcc  bool    // Umgebungsverdeckung
	MouseScale  float64 // Grundfaktor der Mausbewegung
	// Helligkeit als Gamma des Renderers in Prozent, 100 = unverändert (INI 0.50 … 1.50). Die Helligkeit des
	// Spiels selbst (config.cfg) wirkt nur im alten Glide-Pfad.
	Gamma int
}

// defaultD3D entspricht den Vorgaben der Schicht.
var defaultD3D = D3DSettings{
	Fullscreen:  true,
	Resolution:  "",
	MSAA:        4,
	SuperSample: 1,
	Anisotropy:  8,
	VSync:       true,
	Sharpen:     50,
	TexturePack: "hd",
	FontReplace: true,
	AmbientOcc:  false,
	MouseScale:  0.35,
	Gamma:       100,
}

const d3dIniName = "clusterball_d3d.ini"

func d3dIniPath(cfg *AppConfig) string {
	dir := cbExeDir(cfg)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, d3dIniName)
}

// IniFile ist eine INI-Datei mit einem Abschnitt und Zeilen "Schluessel=Wert".
// Wie bei GameConfig bleiben Kommentare und unbekannte Schlüssel unverändert;
// geschrieben werden nur die Zeilen, die der Launcher verwaltet.
type IniFile struct {
	lines []string
	index map[string]int
}

func ParseIni(data string) *IniFile {
	f := &IniFile{
		lines: strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n"),
		index: make(map[string]int),
	}
	for i, line := range f.lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, ";") || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "[") {
			continue
		}
		if k, _, ok := strings.Cut(t, "="); ok {
			k = strings.TrimSpace(k)
			if _, dup := f.index[k]; !dup {
				f.index[k] = i
			}
		}
	}
	return f
}

func (f *IniFile) Get(key string) (string, bool) {
	i, ok := f.index[key]
	if !ok {
		return "", false
	}
	_, v, _ := strings.Cut(f.lines[i], "=")
	return strings.TrimSpace(v), true
}

func (f *IniFile) Int(key string, def int) int {
	v, ok := f.Get(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func (f *IniFile) Float(key string, def float64) float64 {
	v, ok := f.Get(key)
	if !ok {
		return def
	}
	x, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return x
}

func (f *IniFile) Bool(key string, def bool) bool {
	d := 0
	if def {
		d = 1
	}
	return f.Int(key, d) != 0
}

func (f *IniFile) Set(key, value string) {
	line := key + "=" + value
	if i, ok := f.index[key]; ok {
		f.lines[i] = line
		return
	}
	// Wie in GameConfig.Set: vor der abschliessenden Leerzeile einfuegen.
	if n := len(f.lines); n > 0 && strings.TrimSpace(f.lines[n-1]) == "" {
		f.lines[n-1] = line
		f.lines = append(f.lines, "")
		f.index[key] = n - 1
		return
	}
	f.lines = append(f.lines, line)
	f.index[key] = len(f.lines) - 1
}

func (f *IniFile) String() string { return strings.Join(f.lines, "\r\n") }

// Keys liefert die Schlüssel in der Reihenfolge der Datei.
func (f *IniFile) Keys() []string {
	keys := make([]string, 0, len(f.index))
	for i, line := range f.lines {
		t := strings.TrimSpace(line)
		if k, _, ok := strings.Cut(t, "="); ok && !strings.HasPrefix(t, ";") && !strings.HasPrefix(t, "#") {
			if f.index[strings.TrimSpace(k)] == i {
				keys = append(keys, strings.TrimSpace(k))
			}
		}
	}
	return keys
}

// loadD3DIni liefert die mitgelieferte Vorlage mit den Werten aus der INI
// neben dem Spiel. Die Kommentare kommen so immer aus der aktuellen Vorlage;
// Werte - auch von Hand eingetragene Schlüssel, die die Vorlage nicht kennt -
// bleiben erhalten.
func loadD3DIni(path string) *IniFile {
	ini := ParseIni(string(layerINI))
	if path == "" {
		return ini
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ini
	}
	old := ParseIni(string(data))
	for _, k := range old.Keys() {
		v, _ := old.Get(k)
		ini.Set(k, v)
	}
	return ini
}

// readD3DSettings übernimmt die Werte aus der INI in die Konfiguration.
func readD3DSettings(cfg *AppConfig) {
	ini := loadD3DIni(d3dIniPath(cfg))
	d := defaultD3D
	d.Fullscreen = ini.Int("Fullscreen", 1) != 0
	if v, ok := ini.Get("Resolution"); ok {
		d.Resolution = normalizeResolution(v)
	}
	d.MSAA = ini.Int("MSAA", d.MSAA)
	d.SuperSample = ini.Int("SuperSample", d.SuperSample)
	d.Anisotropy = ini.Int("Anisotropy", d.Anisotropy)
	d.VSync = ini.Bool("VSync", d.VSync)
	d.Sharpen = clampInt(int(ini.Float("Sharpen", float64(d.Sharpen)/100)*100+0.5), 0, 100)
	if !ini.Bool("TextureReplace", true) {
		d.TexturePack = ""
	} else if v, ok := ini.Get("TexturePack"); ok && v != "" {
		d.TexturePack = v
	}
	// Das 16-fach-Paket wird nicht mehr ausgeliefert; ältere INIs bekommen das 4-fach-Paket.
	if d.TexturePack == "hd16" {
		d.TexturePack = "hd"
	}
	d.FontReplace = ini.Bool("FontReplace", d.FontReplace)
	d.AmbientOcc = ini.Bool("AmbientOcclusion", d.AmbientOcc)
	d.MouseScale = ini.Float("MouseScale", d.MouseScale)
	d.Gamma = clampInt(int(ini.Float("Gamma", float64(d.Gamma)/100)*100+0.5), 50, 150)
	cfg.D3D = d
}

// writeD3DSettings schreibt die Werte in die INI neben dem Spiel.
func writeD3DSettings(cfg *AppConfig) error {
	path := d3dIniPath(cfg)
	if path == "" {
		return nil
	}
	ini := loadD3DIni(path)
	applyD3DSettings(ini, &cfg.D3D)
	return os.WriteFile(path, []byte(ini.String()), 0644)
}

func applyD3DSettings(ini *IniFile, d *D3DSettings) {
	if d.Fullscreen {
		ini.Set("Fullscreen", "1")
	} else {
		ini.Set("Fullscreen", "0")
	}
	ini.Set("Resolution", d.Resolution)
	ini.Set("MSAA", strconv.Itoa(d.MSAA))
	ini.Set("SuperSample", strconv.Itoa(d.SuperSample))
	ini.Set("Anisotropy", strconv.Itoa(d.Anisotropy))
	ini.Set("VSync", boolStr(d.VSync))
	ini.Set("Sharpen", strconv.FormatFloat(float64(d.Sharpen)/100, 'f', 2, 64))
	if d.TexturePack == "" {
		ini.Set("TextureReplace", "0")
	} else {
		ini.Set("TextureReplace", "1")
		ini.Set("TexturePack", d.TexturePack)
	}
	ini.Set("FontReplace", boolStr(d.FontReplace))
	ini.Set("AmbientOcclusion", boolStr(d.AmbientOcc))
	ini.Set("MouseScale", strconv.FormatFloat(d.MouseScale, 'f', 2, 64))
	ini.Set("Gamma", strconv.FormatFloat(float64(d.Gamma)/100, 'f', 2, 64))
}

// normalizeResolution bringt "1920 x 1080" o.ä. auf "1920x1080"; leer und
// alles Unlesbare heißt native Auflösung.
func normalizeResolution(v string) string {
	v = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(v, " ", ""), "×", "x"))
	var w, h int
	if _, err := fmt.Sscanf(v, "%dx%d", &w, &h); err != nil || w < 320 || h < 240 {
		return ""
	}
	return fmt.Sprintf("%dx%d", w, h)
}

// texturePackAvailable meldet, ob ein Texturpaket im Spielordner liegt.
func texturePackAvailable(cfg *AppConfig, pack string) bool {
	dir := cbExeDir(cfg)
	if dir == "" || pack == "" {
		return pack == ""
	}
	entries, err := os.ReadDir(filepath.Join(dir, "textures", pack))
	return err == nil && len(entries) > 0
}
