package main

import "testing"

func TestMigrationHebtRenderaufloesung(t *testing.T) {
	// Konfiguration aus der Zeit vor configVersion 1: kein Versionseintrag,
	// Renderauflösung auf nGlides Vorgabe "wie im Spiel".
	cfg := AppConfig{GlideResolution: "0"}
	migrateConfig(&cfg)

	if cfg.GlideResolution != "1" {
		t.Errorf("GlideResolution = %q, erwartet \"1\" (wie Desktop)", cfg.GlideResolution)
	}
	if cfg.Version != configVersion {
		t.Errorf("Version = %d, erwartet %d", cfg.Version, configVersion)
	}
}

func TestMigrationRespektiertBewussteWahl(t *testing.T) {
	// Wer nach der Migration bewusst zurückstellt, hat die aktuelle Version
	// gespeichert - das darf nicht erneut überschrieben werden.
	cfg := AppConfig{
		GlideResolution:  "0",
		GraphicalQuality: 0,
		Antialias:        false,
		Version:          configVersion,
	}
	migrateConfig(&cfg)

	if cfg.GlideResolution != "0" {
		t.Errorf("Renderauflösung überschrieben: %q", cfg.GlideResolution)
	}
	if cfg.GraphicalQuality != 0 || cfg.Antialias {
		t.Error("Bildqualität überschrieben, obwohl bewusst gesetzt")
	}
}

func TestMigrationLaesstEigeneAufloesungInRuhe(t *testing.T) {
	cfg := AppConfig{GlideResolution: "1920x1080"}
	migrateConfig(&cfg)

	if cfg.GlideResolution != "1920x1080" {
		t.Errorf("gesetzte Auflösung verändert: %q", cfg.GlideResolution)
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

	if cfg.GraphicalQuality != 2 {
		t.Errorf("GraphicalQuality = %d, erwartet 2 (hoch)", cfg.GraphicalQuality)
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
	if defaultConfig.GlideResolution != "1" {
		t.Errorf("Renderauflösung = %q, erwartet \"1\" (wie Desktop)", defaultConfig.GlideResolution)
	}
	if defaultConfig.Depth != gameDepth {
		t.Errorf("Farbtiefe = %d, erwartet %d", defaultConfig.Depth, gameDepth)
	}
	if !defaultConfig.Antialias || !defaultConfig.TrilinearMipmap ||
		!defaultConfig.Force32BitTex || !defaultConfig.ClampTexture {
		t.Errorf("Qualitätsschalter nicht vorbelegt: %+v", defaultConfig)
	}
	if defaultConfig.GraphicalQuality != 2 || defaultConfig.Brightness != 90 {
		t.Errorf("Detailstufe/Helligkeit falsch: %d / %d",
			defaultConfig.GraphicalQuality, defaultConfig.Brightness)
	}
}

// Nur 16 Bit darf angeboten werden.
func TestNurEineFarbtiefe(t *testing.T) {
	if len(depthValues) != 1 || depthValues[0] != gameDepth {
		t.Errorf("depthValues = %v, erwartet [%d]", depthValues, gameDepth)
	}
	if len(depthLabels()) != len(depthValues) {
		t.Errorf("%d Beschriftungen zu %d Werten", len(depthLabels()), len(depthValues))
	}
}
