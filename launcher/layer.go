package main

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
)

// Die Direct3D-Schicht: eine opengl32.dll, die Clusterball.exe neben sich
// lädt und die die OpenGL-Aufrufe des Spiels auf Direct3D 11 umsetzt (Quelle
// im Ordner modern-renderer/ des Repositorys, gebaut von build.cmd). Der Launcher
// bringt sie mit und legt sie vor jedem Start neben das Spiel.
//
//go:embed layer/opengl32.dll
var layerDLL []byte

// Vorlage der Einstellungsdatei der Schicht, mit allen Kommentaren.
//
//go:embed layer/clusterball_d3d.ini
var layerINI []byte

// legacyRenderFiles sind die Grafiktreiber älterer Pakete, die neben der
// Direct3D-Schicht nichts mehr zu tun haben: nGlide (glide2x.dll) mit seinem
// 3dfx-Startlogo. Die Schicht verhindert zwar, dass das Spiel Glide lädt -
// liegen bleiben sollen sie trotzdem nicht. Eine fremde opengl32.dll (Mesa)
// wird von installLayer ersetzt.
var legacyRenderFiles = []string{"glide2x.dll", "3DfxSpl2.dll"}

// installLayer legt die mitgelieferte opengl32.dll neben das Spiel und
// entfernt die alten Glide-Dateien.
func installLayer(gameDir string) error {
	for _, name := range legacyRenderFiles {
		if err := os.Remove(filepath.Join(gameDir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	path := filepath.Join(gameDir, "opengl32.dll")
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, layerDLL) {
		return nil
	}
	return os.WriteFile(path, layerDLL, 0644)
}

// prepareGame bringt das Spielverzeichnis vor dem Start auf Stand: Schicht,
// ihre Einstellungen, das Icon von Clusterball.exe und der Schrift-Ersatz.
// Fehler bei Icon und Schrift-Ersatz sind nicht fatal - das Spiel behält
// dann sein altes Icon bzw. nutzt seine eigene Schrift.
func prepareGame(cfg *AppConfig) (fontErr, err error) {
	dir := cbExeDir(cfg)
	if dir == "" {
		return nil, nil
	}
	if err := installLayer(dir); err != nil {
		return nil, err
	}
	ensureGameIcon(cfg.CBExePath)
	installDocs(dir) // Readme, Lizenz, Notice (docs.go)
	// Ton von ClusterForce vor der INI, die auf die Dateien verweist.
	if err := installCFAudio(dir, &cfg.D3D); err != nil {
		return nil, err
	}
	if err := writeD3DSettings(cfg); err != nil {
		return nil, err
	}
	if cfg.D3D.FontReplace {
		fontErr = ensureFontAtlas(dir)
	}
	return fontErr, nil
}
