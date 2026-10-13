package main

// Sprachtabellen der Oberfläche.
//
// currentLang ist bewusst paketweit: auch launchCB und launchDedicatedHost
// zeigen Meldungen an, laufen aber ohne Fensterreferenz (z.B. beim Start mit
// -d oder -g).
//
// Fehlt ein Schlüssel in textsEN, greift automatisch der deutsche Text – eine
// unvollständige Übersetzung führt also nie zu leeren Beschriftungen.
var currentLang = "de"

func setLang(lang string) {
	if lang == "en" {
		currentLang = "en"
		return
	}
	currentLang = "de"
}

func tr(key string) string {
	if currentLang == "en" {
		if s, ok := textsEN[key]; ok {
			return s
		}
	}
	if s, ok := textsDE[key]; ok {
		return s
	}
	return key
}

var textsDE = map[string]string{
	"tab.game":     "▶  Spiel",
	"tab.settings": "⚙  Einstellungen",
	"tab.help":     "?  Hilfe",

	"acc.player":      "Spieler:",
	"acc.playerCue":   "clusterball.de Account",
	"acc.password":    "Passwort:",
	"acc.passwordCue": "Passwort",
	"acc.register":    "Jetzt registrieren",
	"acc.registerTip": "Kostenlosen clusterball.de-Account anlegen – ohne Account sind nur LAN-Spiele möglich",
	"acc.hint":        "ohne Account:\nnur LAN-Spiele",

	"act.start":     "▶  Spiel starten",
	"act.dedicated": "•  Dedicated Host",
	"act.ownIP":     " Eigene IP ",
	"act.detectTip": "Externe IP jetzt neu erkennen",
	"act.external":  "externe IP verwenden",

	"srv.title":      "ONLINE SERVER",
	"srv.titleN":     "ONLINE SERVER  (%d)",
	"srv.titleNone":  "ONLINE SERVER  (keine)",
	"srv.refresh":    "↺ Aktualisieren",
	"srv.colName":    "Server-Name",
	"srv.colPlayers": "Spieler",
	"srv.colMap":     "Karte",
	"srv.colType":    "Typ",
	"srv.hint":       "Doppelklick zum Beitreten  ·  Auto-Refresh alle 10s",
	"srv.dedicated":  "Dedicated",

	"set.graphics": "Grafik (Direct3D 11)",
	"set.network":  "Netzwerk",
	"set.master":   "Master-Server IP:",
	"set.language": "Sprache",
	"set.save":     "✓  Einstellungen speichern",
	"set.saveTip":  "Speichert nach %s",

	"set.display":       "Anzeige:",
	"set.fullscreen":    "Vollbild",
	"set.window":        "Fenster",
	"set.resolution":    "Auflösung:",
	"set.resolutionTip": "Auflösung, in der gezeichnet wird. „Nativ“ nimmt die des Monitors und ist am schärfsten; eine kleinere wird im Vollbild auf den Monitor gestreckt.",
	"set.resNative":     "Nativ (Monitor)",
	"set.msaa":          "Kantenglättung:",
	"set.ss":            "Supersampling:",
	"set.ssTip":         "Zeichnet in doppelter Auflösung und rechnet herunter – sehr glatte Kanten, braucht aber deutlich mehr Grafikleistung.",
	"set.aniso":         "Anisotrope Filterung:",
	"set.off":           "aus",
	"set.texPack":       "Texturen:",
	"set.texPackTip":    "Hochaufgelöste Texturpakete liegen im Spielordner unter textures\\hd bzw. textures\\hd16. Fehlt eine Textur im 16-fach-Paket, wird die 4-fach-Fassung genommen.",
	"set.texOrig":       "Original",
	"set.texHD4":        "HD (4-fach)",
	"set.texHD16":       "HD (16-fach)",
	"set.sharpen":       "Schärfe:",
	"set.vsync":         "Vertikale Synchronisation",
	"set.font":          "Scharfe Menü- und HUD-Schrift",
	"set.fontTip":       "Ersetzt die 8 × 8 Pixel kleine Schrift des Spiels durch eine hochaufgelöste aus der Bahnschrift von Windows.",
	"set.ao":            "Umgebungsverdeckung",
	"set.aoTip":         "Schattiert Ecken und Kanten. Zeigt auf dem groben Gelände dessen Facetten, deshalb standardmäßig aus.",
	"set.d3dHint":       "Alt+Enter wechselt im Spiel zwischen Vollbild und Fenster",

	"set.game":       "Spiel",
	"set.quality":    "Detailstufe:",
	"set.qLow":       "niedrig",
	"set.qMid":       "mittel",
	"set.qHigh":      "hoch",
	"set.brightness": "Helligkeit:",
	"set.mouse":      "Maus:",
	"set.mouseTip":   "Empfindlichkeit der Maus im Spiel; die Zeigergeschwindigkeit von Windows kommt hinzu.",
	"set.gfxHint":    "wirkt beim nächsten Spielstart",

	"st.ready":       "Bereit",
	"st.starting":    "Clusterball wird gestartet …",
	"st.dediStart":   "Dedicated Host wird gestartet …",
	"st.dediRunning": "Dedicated Host läuft – Heartbeat aktiv",
	"st.loading":     "Serverliste wird geladen …",
	"st.listError":   "Serverliste nicht erreichbar: %s",
	"st.noServers":   "Keine Server online",
	"st.nServers1":   "1 Server online",
	"st.nServers":    "%d Server online",
	"st.saved":       "Einstellungen gespeichert ✔",
	"st.ipDetect":    "Externe IP wird ermittelt …",
	"st.ipFailed":    "IP-Erkennung fehlgeschlagen: %s",
	"st.ipOK":        "Externe IP: %s",
	"st.joining":     "Starte für „%s“ …",
	"st.cfgFailed":   "Konfiguration nicht gespeichert: %s",

	"dlg.error":     "Fehler",
	"dlg.noPath":    "Der Spielordner konnte nicht ermittelt werden.",
	"dlg.notFound":  "Clusterball.exe nicht gefunden:\n%s\n\nDer Launcher gehört in den Spielordner, neben die Clusterball.exe.",
	"dlg.heartbeat": "Heartbeat-Fehler",
	"dlg.gameCfg":   "config.cfg des Spiels konnte nicht geschrieben werden:\n%s",
	"dlg.d3dCfg":    "clusterball_d3d.ini konnte nicht geschrieben werden:\n%s",
	"dlg.layer":     "Die Direct3D-Schicht (opengl32.dll) konnte nicht ins Spielverzeichnis gelegt werden - läuft das Spiel noch?\n%s",
	"dlg.font":      "Die scharfe Schrift konnte nicht erzeugt werden, das Spiel nutzt seine eigene:\n%s",

	"sb.force": "Get ClusterForce!",
	"sb.brand": "clusterball.de · XDREAM GAMES NETWORK",
}

var textsEN = map[string]string{
	"tab.game":     "▶  Game",
	"tab.settings": "⚙  Settings",
	"tab.help":     "?  Help",

	"acc.player":      "Player:",
	"acc.playerCue":   "clusterball.de account",
	"acc.password":    "Password:",
	"acc.passwordCue": "Password",
	"acc.register":    "Create account",
	"acc.registerTip": "Create a free clusterball.de account – without one, only LAN games are possible",
	"acc.hint":        "no account:\nLAN games only",

	"act.start":     "▶  Start game",
	"act.dedicated": "•  Dedicated host",
	"act.ownIP":     " Own IP ",
	"act.detectTip": "Detect external IP again",
	"act.external":  "use external IP",

	"srv.title":      "ONLINE SERVERS",
	"srv.titleN":     "ONLINE SERVERS  (%d)",
	"srv.titleNone":  "ONLINE SERVERS  (none)",
	"srv.refresh":    "↺ Refresh",
	"srv.colName":    "Server name",
	"srv.colPlayers": "Players",
	"srv.colMap":     "Map",
	"srv.colType":    "Type",
	"srv.hint":       "Double-click to join  ·  auto-refresh every 10s",
	"srv.dedicated":  "Dedicated",

	"set.graphics": "Graphics (Direct3D 11)",
	"set.network":  "Network",
	"set.master":   "Master server IP:",
	"set.language": "Language",
	"set.save":     "✓  Save settings",
	"set.saveTip":  "Saves to %s",

	"set.display":       "Display:",
	"set.fullscreen":    "Fullscreen",
	"set.window":        "Window",
	"set.resolution":    "Resolution:",
	"set.resolutionTip": "The resolution the game is drawn at. “Native” uses the monitor's and is the sharpest; a smaller one is stretched to the monitor in fullscreen.",
	"set.resNative":     "Native (monitor)",
	"set.msaa":          "Anti-aliasing:",
	"set.ss":            "Supersampling:",
	"set.ssTip":         "Draws at twice the resolution and scales down – very smooth edges, but needs much more graphics power.",
	"set.aniso":         "Anisotropic filtering:",
	"set.off":           "off",
	"set.texPack":       "Textures:",
	"set.texPackTip":    "High-resolution texture packs live in the game folder under textures\\hd and textures\\hd16. A texture missing from the 16x pack uses its 4x version.",
	"set.texOrig":       "Original",
	"set.texHD4":        "HD (4x)",
	"set.texHD16":       "HD (16x)",
	"set.sharpen":       "Sharpness:",
	"set.vsync":         "Vertical synchronisation",
	"set.font":          "Sharp menu and HUD font",
	"set.fontTip":       "Replaces the game's tiny 8 × 8 pixel font with a high-resolution one made from Windows' Bahnschrift.",
	"set.ao":            "Ambient occlusion",
	"set.aoTip":         "Shades corners and creases. On the coarse terrain it shows the facets, hence off by default.",
	"set.d3dHint":       "Alt+Enter switches between fullscreen and window in the game",

	"set.game":       "Game",
	"set.quality":    "Detail level:",
	"set.qLow":       "low",
	"set.qMid":       "medium",
	"set.qHigh":      "high",
	"set.brightness": "Brightness:",
	"set.mouse":      "Mouse:",
	"set.mouseTip":   "Mouse sensitivity in the game; the Windows pointer speed is applied on top.",
	"set.gfxHint":    "applies on next game start",

	"st.ready":       "Ready",
	"st.starting":    "Starting Clusterball …",
	"st.dediStart":   "Starting dedicated host …",
	"st.dediRunning": "Dedicated host running – heartbeat active",
	"st.loading":     "Loading server list …",
	"st.listError":   "Server list unreachable: %s",
	"st.noServers":   "No servers online",
	"st.nServers1":   "1 server online",
	"st.nServers":    "%d servers online",
	"st.saved":       "Settings saved ✔",
	"st.ipDetect":    "Detecting external IP …",
	"st.ipFailed":    "IP detection failed: %s",
	"st.ipOK":        "External IP: %s",
	"st.joining":     "Starting for “%s” …",
	"st.cfgFailed":   "Could not save configuration: %s",

	"dlg.error":     "Error",
	"dlg.noPath":    "Could not determine the game folder.",
	"dlg.notFound":  "Clusterball.exe not found:\n%s\n\nThe launcher belongs in the game folder, next to Clusterball.exe.",
	"dlg.heartbeat": "Heartbeat error",
	"dlg.gameCfg":   "Could not write the game's config.cfg:\n%s",
	"dlg.d3dCfg":    "Could not write clusterball_d3d.ini:\n%s",
	"dlg.layer":     "Could not place the Direct3D layer (opengl32.dll) in the game folder - is the game still running?\n%s",
	"dlg.font":      "Could not create the sharp font, the game uses its own:\n%s",

	"sb.force": "Get ClusterForce!",
	"sb.brand": "clusterball.de · XDREAM GAMES NETWORK",
}
