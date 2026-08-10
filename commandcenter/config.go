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
const configVersion = 2

// AppConfig hält alle Einstellungen der Anwendung.
type AppConfig struct {
	Version int // zuletzt angewandte configVersion

	CBExePath        string
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
	Renderer         string
	Resolution       string
	Width            int
	Height           int
	BufferDepth      string
	ClampTexture     bool
	BlobbMode        bool

	// Grafik des Spiels – Quelle ist die config.cfg neben der Clusterball.exe
	Depth            int // 16 oder 32 Bit
	GraphicalQuality int // 0 = niedrig … 2 = hoch
	Brightness       int // 0 … 100
	Antialias        bool
	TrilinearMipmap  bool
	Force32BitTex    bool

	// nGlide (Glide-Wrapper), gesetzt als Umgebungsvariablen beim Spielstart
	GlideResolution string // "0" = vom Spiel, "1" = Desktop, sonst z.B. "1920x1080"
	GlideAspect     string // "0" = Bildschirm füllen, "1" = wie Spiel, "4:3"
	GlideVSync      bool
	GlideSplash     bool
}

// defaultConfig übernimmt die Vorgaben des alten XDreamCC (XDreamCC.ini,
// Abschnitt [Default]): hres/vres 1024x768, hltimer 20, Depth/Resolution
// "standard" bzw. "userdef.", ClampTexture und BlobbMode aus. Damit ist
// XDreamCC.ini nicht mehr nötig – der Launcher startet auch ohne jede
// vorhandene Konfiguration sinnvoll.
//
// Zwei Werte weichen bewusst ab: Renderer ist 3dfx (statt Renderer=1) und
// die externe IP ist standardmäßig an (statt UseExternal=0).
var defaultConfig = AppConfig{
	ServerMode:       "arena",
	ServerIP:         "178.115.244.173",
	Language:         "de",
	HostlistDuration: 20,
	Renderer:         "3dfx",
	Resolution:       "userdef.",
	Width:            gameResW,
	Height:           gameResH,
	BufferDepth:      "standard",
	UseExternalIP:    true,
	ClampTexture:     true,
	BlobbMode:        false,

	// Empfohlene Bildqualität: alles an, was die Darstellung verbessert.
	// Die Farbtiefe steht fest, siehe gameDepth.
	Depth:            gameDepth,
	GraphicalQuality: 2, // hoch
	Brightness:       90,
	Antialias:        true,
	TrilinearMipmap:  true,
	Force32BitTex:    true,

	// Renderauflösung bewusst auf "wie Desktop" statt auf nGlides eigene
	// Vorgabe "wie im Spiel": letztere ließe den Wrapper in 640 × 480
	// zeichnen, also genau in der Auflösung, deren Grenze wir umgehen wollen.
	GlideResolution: "1",
	GlideAspect:     "0",
	GlideVSync:      false,
	GlideSplash:     true,
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

// configPath gibt den Pfad zur Konfigurationsdatei zurück.
func configPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "xdreamcc.cfg"
	}
	return filepath.Join(filepath.Dir(exe), "xdreamcc.cfg")
}

// cbExeDir gibt das Verzeichnis der Clusterball-EXE zurück.
func cbExeDir(cfg *AppConfig) string {
	if cfg.CBExePath == "" {
		return ""
	}
	return filepath.Dir(cfg.CBExePath)
}

// LoadConfig lädt die Konfiguration aus der Datei.
func LoadConfig() AppConfig {
	cfg := defaultConfig

	f, err := os.Open(configPath())
	if err != nil {
		return cfg
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
		case "cb_exe":
			cfg.CBExePath = v
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
		case "renderer":
			cfg.Renderer = v
		case "resolution":
			cfg.Resolution = v
		case "width":
			cfg.Width, _ = strconv.Atoi(v)
		case "height":
			cfg.Height, _ = strconv.Atoi(v)
		case "buffer_depth":
			cfg.BufferDepth = v
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
		case "glide_resolution":
			cfg.GlideResolution = v
		case "glide_aspect":
			cfg.GlideAspect = v
		case "glide_vsync":
			cfg.GlideVSync = v == "1"
		case "glide_splash":
			cfg.GlideSplash = v == "1"
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

	// Clusterball.exe wird immer neben dem Launcher gesucht. Das gewinnt
	// gegen einen gespeicherten Pfad, damit ein verschobenes oder kopiertes
	// Spielverzeichnis nicht auf die alte Installation zeigt. Nur wenn dort
	// nichts liegt, bleibt der konfigurierte Pfad als Rückfallebene stehen.
	if local := localCBExe(); local != "" {
		if _, err := os.Stat(local); err == nil {
			cfg.CBExePath = local
		}
	}

	// Die Grafikwerte des Spiels kommen aus dessen eigener config.cfg – die
	// gewinnt gegen alles, was hier gespeichert ist. Sonst zeigt der Launcher
	// Werte an, die im Spiel gar nicht gelten.
	readGameSettings(&cfg)
	migrateConfig(&cfg)

	// Immer das Maximum bzw. die einzige Farbtiefe, die das Spiel kennt -
	// unabhaengig davon, was in einer aelteren Konfiguration steht.
	cfg.Width, cfg.Height = gameResW, gameResH
	cfg.Depth = gameDepth
	return cfg
}

// migrateConfig hebt gespeicherte Konfigurationen auf geänderte Vorgaben an.
//
// Version 1: Die Renderauflösung des Wrappers stand anfangs auf nGlides
// eigener Vorgabe "wie im Spiel" – damit zeichnet er in 640 × 480, also genau
// in der Grenze, die umgangen werden soll. Ein Eintrag aus dieser Zeit wird
// einmalig auf "wie Desktop" gehoben. Wer die Einstellung danach bewusst
// zurückstellt, behält sie: die Version steht dann bereits auf 1.
// Version 2: Die Bildqualität wird einmalig auf die empfohlene Vorgabe
// gehoben. Nötig ist das, weil diese Werte aus der config.cfg des Spiels
// kommen – ein geänderter Vorgabewert im Launcher würde sie sonst nie
// erreichen. Auch hier gilt: was danach umgestellt wird, bleibt so.
func migrateConfig(cfg *AppConfig) {
	if cfg.Version < 1 {
		if cfg.GlideResolution == "0" {
			cfg.GlideResolution = "1"
		}
	}
	if cfg.Version < 2 {
		cfg.GraphicalQuality = defaultConfig.GraphicalQuality
		cfg.Brightness = defaultConfig.Brightness
		cfg.Antialias = defaultConfig.Antialias
		cfg.TrilinearMipmap = defaultConfig.TrilinearMipmap
		cfg.Force32BitTex = defaultConfig.Force32BitTex
		cfg.ClampTexture = defaultConfig.ClampTexture
	}
	cfg.Version = configVersion
}

// SaveConfig speichert die Konfiguration in die Datei.
func SaveConfig(cfg *AppConfig) error {
	f, err := os.Create(configPath())
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	fmt.Fprintf(w, "config_version=%d\n", configVersion)
	fmt.Fprintf(w, "cb_exe=%s\n", cfg.CBExePath)
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
	fmt.Fprintf(w, "renderer=%s\n", cfg.Renderer)
	fmt.Fprintf(w, "resolution=%s\n", cfg.Resolution)
	fmt.Fprintf(w, "width=%d\n", cfg.Width)
	fmt.Fprintf(w, "height=%d\n", cfg.Height)
	fmt.Fprintf(w, "buffer_depth=%s\n", cfg.BufferDepth)
	fmt.Fprintf(w, "clamp_texture=%s\n", boolStr(cfg.ClampTexture))
	fmt.Fprintf(w, "blobb_mode=%s\n", boolStr(cfg.BlobbMode))
	fmt.Fprintf(w, "depth_bits=%d\n", cfg.Depth)
	fmt.Fprintf(w, "graphical_quality=%d\n", cfg.GraphicalQuality)
	fmt.Fprintf(w, "brightness=%d\n", cfg.Brightness)
	fmt.Fprintf(w, "antialias=%s\n", boolStr(cfg.Antialias))
	fmt.Fprintf(w, "trilinear_mipmap=%s\n", boolStr(cfg.TrilinearMipmap))
	fmt.Fprintf(w, "force_32bit_texture=%s\n", boolStr(cfg.Force32BitTex))
	fmt.Fprintf(w, "glide_resolution=%s\n", cfg.GlideResolution)
	fmt.Fprintf(w, "glide_aspect=%s\n", cfg.GlideAspect)
	fmt.Fprintf(w, "glide_vsync=%s\n", boolStr(cfg.GlideVSync))
	fmt.Fprintf(w, "glide_splash=%s\n", boolStr(cfg.GlideSplash))
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
