package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// configVersion steigt, wenn sich eine Vorgabe ändert, die auch bereits
// gespeicherte Konfigurationen übernehmen sollen. Siehe migrateConfig.
const configVersion = 3

// AppConfig hält alle Einstellungen der Anwendung.
type AppConfig struct {
	Version int // zuletzt angewandte configVersion

	CBExePath        string // immer die Clusterball.exe neben dem Launcher, siehe LoadConfig
	ServerMode       string // "arena" oder "cbcom"
	ServerIP         string
	HostIP           string
	OwnIP            string
	UseExternalIP    bool
	DedicatedHost    bool
	Language         string // "de" oder "en"
	AccountName      string // clusterball.de Account
	AccountPassword  string // clusterball.de Passwort
	PlayerName       [4]string
	HotkeyMinimize   int
	HotkeyHostlist   int
	HostlistDuration int
	ClampTexture     bool
	BlobbMode        bool

	// Grafik des Spiels – Quelle ist die config.cfg neben der Clusterball.exe
	Depth            int // Farbtiefe, fest auf gameDepth
	GraphicalQuality int // graphicalQuality des Spiels, steht fest auf gameQuality
	Brightness       int // 0 … 100
	Antialias        bool
	TrilinearMipmap  bool
	Force32BitTex    bool

	// Direct3D-Schicht – Quelle ist die clusterball_d3d.ini neben der
	// Clusterball.exe, nicht diese Datei
	D3D D3DSettings

	// Tastenbelegung: Aktion der config.cfg -> Tastenname des Spiels. Quelle ist
	// die config.cfg, nicht diese Datei; siehe controls.go.
	Bindings map[string]string
}

// defaultConfig übernimmt die Vorgaben des alten XDreamCC (XDreamCC.ini,
// Abschnitt [Default]): hltimer 20, ClampTexture und BlobbMode aus. Damit ist
// XDreamCC.ini nicht mehr nötig – der Launcher startet auch ohne jede
// vorhandene Konfiguration sinnvoll. Abweichend ist die externe IP
// standardmäßig an (statt UseExternal=0).
var defaultConfig = AppConfig{
	ServerMode:       "arena",
	ServerIP:         "178.115.244.173",
	Language:         "", // ohne gespeicherte Wahl: Sprache von Windows, siehe LoadConfig
	HostlistDuration: 20,
	UseExternalIP:    true,
	ClampTexture:     true,
	BlobbMode:        false,

	// Empfohlene Bildqualität: alles an, was die Darstellung verbessert.
	// Die Farbtiefe steht fest, siehe gameDepth.
	Depth:            gameDepth,
	GraphicalQuality: gameQuality,
	Brightness:       90,
	Antialias:        true,
	TrilinearMipmap:  true,
	Force32BitTex:    true,

	D3D: defaultD3D,
}

// launcherDir gibt das Verzeichnis der Launcher-EXE zurück.
func launcherDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

// localCBExe ist der Pfad zur Clusterball.exe neben dem Launcher – der
// Normalfall, weil der Launcher im Spielverzeichnis liegt.
func localCBExe() string {
	dir := launcherDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "Clusterball.exe")
}

// Die Einstellungen des Launchers liegen neben ihm in clusterball.ini.
// Frühere Versionen schrieben sie nach xdreamcc.cfg: die wird gelesen,
// solange es noch keine clusterball.ini gibt, und nach dem ersten
// Speichern gelöscht.
const (
	configName       = "clusterball.ini"
	legacyConfigName = "xdreamcc.cfg"
)

// appDir ist das Verzeichnis des Launchers.
func appDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// configPath gibt den Pfad zur Konfigurationsdatei zurück.
func configPath() string { return filepath.Join(appDir(), configName) }

// configToRead nennt die Datei, aus der die Einstellungen kommen: die
// clusterball.ini, sonst die alte xdreamcc.cfg, sonst keine ("").
func configToRead(dir string) string {
	for _, n := range []string{configName, legacyConfigName} {
		if p := filepath.Join(dir, n); fileExists(p) {
			return p
		}
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// cbExeDir gibt das Verzeichnis der Clusterball-EXE zurück.
func cbExeDir(cfg *AppConfig) string {
	if cfg.CBExePath == "" {
		return ""
	}
	return filepath.Dir(cfg.CBExePath)
}

// LoadConfig lädt die Konfiguration. Fehlt die Datei, gelten die Vorgaben -
// Spielverzeichnis, config.cfg und clusterball_d3d.ini werden trotzdem
// gelesen.
func LoadConfig() AppConfig {
	cfg := defaultConfig
	readAppConfig(&cfg)
	// Beim ersten Start folgt die Sprache der Windows-Oberflaeche; danach gilt,
	// was der Umschalter gespeichert hat.
	if cfg.Language != "de" && cfg.Language != "en" {
		cfg.Language = systemLanguage()
	}

	// Clusterball.exe liegt immer neben dem Launcher; einen anderen Ort gibt
	// es nicht. Ein in älteren Konfigurationen gespeicherter Pfad wird nicht
	// mehr gelesen, damit ein verschobenes oder kopiertes Spielverzeichnis
	// nie auf eine andere Installation zeigt.
	cfg.CBExePath = localCBExe()

	// Die Grafikwerte des Spiels kommen aus dessen eigener config.cfg – die
	// gewinnt gegen alles, was hier gespeichert ist. Sonst zeigt der Launcher
	// Werte an, die im Spiel gar nicht gelten.
	readGameSettings(&cfg)
	readD3DSettings(&cfg)
	migrateConfig(&cfg)

	// Immer die einzige Farbtiefe, mit der das Spiel unter der Direct3D-Schicht
	// startet - unabhaengig davon, was in einer aelteren Konfiguration steht.
	cfg.Depth = gameDepth
	cfg.GraphicalQuality = gameQuality
	return cfg
}

// readAppConfig liest die Einstellungen des Launchers, sofern es sie gibt.
func readAppConfig(cfg *AppConfig) { readAppConfigFrom(cfg, configToRead(appDir())) }

func readAppConfigFrom(cfg *AppConfig, path string) {
	if path == "" {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "server_mode":
			cfg.ServerMode = v
		case "server_ip":
			cfg.ServerIP = v
		case "host_ip":
			cfg.HostIP = v
		case "own_ip":
			cfg.OwnIP = v
		case "use_external_ip":
			cfg.UseExternalIP = v == "1"
		case "language":
			cfg.Language = v
		case "account_name":
			cfg.AccountName = v
		case "account_password":
			cfg.AccountPassword = v
		case "hotkey_minimize":
			cfg.HotkeyMinimize, _ = strconv.Atoi(v)
		case "hotkey_hostlist":
			cfg.HotkeyHostlist, _ = strconv.Atoi(v)
		case "hostlist_duration":
			cfg.HostlistDuration, _ = strconv.Atoi(v)
		case "clamp_texture":
			cfg.ClampTexture = v == "1"
		case "blobb_mode":
			cfg.BlobbMode = v == "1"
		case "depth_bits":
			cfg.Depth, _ = strconv.Atoi(v)
		case "graphical_quality":
			cfg.GraphicalQuality, _ = strconv.Atoi(v)
		case "brightness":
			cfg.Brightness, _ = strconv.Atoi(v)
		case "antialias":
			cfg.Antialias = v == "1"
		case "trilinear_mipmap":
			cfg.TrilinearMipmap = v == "1"
		case "force_32bit_texture":
			cfg.Force32BitTex = v == "1"
		case "config_version":
			cfg.Version, _ = strconv.Atoi(v)
		default:
			for i := 0; i < 4; i++ {
				if k == fmt.Sprintf("player%d", i+1) {
					cfg.PlayerName[i] = v
				}
			}
		}
	}
}

// migrateConfig hebt gespeicherte Konfigurationen auf geänderte Vorgaben an.
//
// Version 1 betraf die Renderauflösung des früheren Glide-Wrappers und ist
// mit ihm entfallen.
// Version 2: Die Bildqualität wird einmalig auf die empfohlene Vorgabe
// gehoben. Nötig ist das, weil diese Werte aus der config.cfg des Spiels
// kommen – ein geänderter Vorgabewert im Launcher würde sie sonst nie
// erreichen. Was danach umgestellt wird, bleibt so.
// Version 3: Tastenbelegung WASD + Maus, siehe unten.
func migrateConfig(cfg *AppConfig) {
	if cfg.Version < 2 {
		cfg.GraphicalQuality = defaultConfig.GraphicalQuality
		cfg.Brightness = defaultConfig.Brightness
		cfg.Antialias = defaultConfig.Antialias
		cfg.TrilinearMipmap = defaultConfig.TrilinearMipmap
		cfg.Force32BitTex = defaultConfig.Force32BitTex
		cfg.ClampTexture = defaultConfig.ClampTexture
	}
	// Version 3: WASD + Maus statt der Auslieferungsbelegung - aber nur, wenn
	// noch genau die Auslieferungsbelegung eingestellt ist.
	if cfg.Version < 3 && len(cfg.Bindings) > 0 && sameBindings(cfg.Bindings, presetClassic) {
		cfg.Bindings = presetIn(presetWASD, bindingsLang(cfg.Bindings))
	}
	cfg.Version = configVersion
}

// SaveConfig speichert die Konfiguration in die Datei.
func SaveConfig(cfg *AppConfig) error { return saveConfigIn(cfg, appDir()) }

// saveConfigIn schreibt clusterball.ini in dir und löscht danach eine alte
// xdreamcc.cfg - erst wenn die neue Datei vollständig geschrieben ist.
func saveConfigIn(cfg *AppConfig, dir string) error {
	if err := writeConfig(cfg, filepath.Join(dir, configName)); err != nil {
		return err
	}
	if old := filepath.Join(dir, legacyConfigName); fileExists(old) {
		os.Remove(old)
	}
	return nil
}

func writeConfig(cfg *AppConfig, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	fmt.Fprintf(w, "# Clusterball Re-Enhanced - Einstellungen des Launchers (ClusterballCC.exe)\n")
	fmt.Fprintf(w, "config_version=%d\n", configVersion)
	fmt.Fprintf(w, "server_mode=%s\n", cfg.ServerMode)
	fmt.Fprintf(w, "server_ip=%s\n", cfg.ServerIP)
	fmt.Fprintf(w, "host_ip=%s\n", cfg.HostIP)
	fmt.Fprintf(w, "own_ip=%s\n", cfg.OwnIP)
	fmt.Fprintf(w, "use_external_ip=%s\n", boolStr(cfg.UseExternalIP))
	fmt.Fprintf(w, "language=%s\n", cfg.Language)
	fmt.Fprintf(w, "account_name=%s\n", cfg.AccountName)
	fmt.Fprintf(w, "account_password=%s\n", cfg.AccountPassword)
	fmt.Fprintf(w, "hotkey_minimize=%d\n", cfg.HotkeyMinimize)
	fmt.Fprintf(w, "hotkey_hostlist=%d\n", cfg.HotkeyHostlist)
	fmt.Fprintf(w, "hostlist_duration=%d\n", cfg.HostlistDuration)
	fmt.Fprintf(w, "clamp_texture=%s\n", boolStr(cfg.ClampTexture))
	fmt.Fprintf(w, "blobb_mode=%s\n", boolStr(cfg.BlobbMode))
	fmt.Fprintf(w, "depth_bits=%d\n", cfg.Depth)
	fmt.Fprintf(w, "graphical_quality=%d\n", cfg.GraphicalQuality)
	fmt.Fprintf(w, "brightness=%d\n", cfg.Brightness)
	fmt.Fprintf(w, "antialias=%s\n", boolStr(cfg.Antialias))
	fmt.Fprintf(w, "trilinear_mipmap=%s\n", boolStr(cfg.TrilinearMipmap))
	fmt.Fprintf(w, "force_32bit_texture=%s\n", boolStr(cfg.Force32BitTex))
	for i, n := range cfg.PlayerName {
		fmt.Fprintf(w, "player%d=%s\n", i+1, n)
	}
	return w.Flush()
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
