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

	"set.program":    "Clusterball-Programm",
	"set.browse":     "Durchsuchen …",
	"set.browseTip":  "Clusterball.exe auswählen",
	"set.graphics":   "Grafik",
	"set.renderer":   "Renderer:",
	"set.resolution": "Auflösung:",
	"set.network":    "Netzwerk",
	"set.master":     "Master-Server IP:",
	"set.language":   "Sprache",
	"set.save":       "✓  Einstellungen speichern",
	"set.saveTip":    "Speichert nach %s",

	"set.depth":      "Farbtiefe:",
	"set.depth16":    "16 Bit",
	"set.depthTip":   "Glide arbeitet mit einem 16-Bit-Bildspeicher – eine andere Farbtiefe bietet die Schnittstelle nicht an. Feinere Texturen bringt stattdessen der Schalter „32-Bit-Texturen“.",
	"set.quality":    "Detailstufe:",
	"set.qLow":       "niedrig",
	"set.qMid":       "mittel",
	"set.qHigh":      "hoch",
	"set.brightness": "Helligkeit:",
	"set.antialias":  "Kantenglättung",
	"set.trilinear":  "Trilineare Filterung",
	"set.tex32":      "32-Bit-Texturen",
	"set.clamp":      "Texturkanten klemmen",
	"set.gfxHint":    "wirkt beim nächsten Spielstart",

	"set.resolutionMax": "(Maximum des Spiels)",
	"set.resolutionTip": "Fest auf die höchste Auflösung gesetzt, die Clusterball selbst kennt – das Spiel bietet nur 640 × 480, 800 × 600 und 1024 × 768. Für ein schärferes Bild die Renderauflösung des Wrappers erhöhen.",

	"set.glide":         "Glide-Wrapper (nGlide)",
	"set.glideRes":      "Renderauflösung:",
	"set.glideResTip":   "Auflösung, in der nGlide die 3D-Szene tatsächlich zeichnet – unabhängig von der Auflösung, die das Spiel anfordert. Höher heißt schärferes Bild, kein Hochskalieren.",
	"set.glideResApp":   "wie im Spiel",
	"set.glideResDsk":   "wie Desktop",
	"set.glideAspect":   "Seitenverhältnis:",
	"set.glideAspFit":   "Bildschirm füllen",
	"set.glideAspApp":   "wie im Spiel",
	"set.glideAsp43":    "4:3",
	"set.glideVSync":    "Vertikale Synchronisation",
	"set.glideSplash":   "3dfx-Logo beim Start",
	"set.glideOnly3dfx": "nur wirksam mit Renderer 3dfx",

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
	"st.exeMissing":  "Clusterball.exe nicht gefunden – bitte unten eintragen",
	"st.joining":     "Starte für „%s“ …",
	"st.cfgFailed":   "Konfiguration nicht gespeichert: %s",

	"dlg.error":     "Fehler",
	"dlg.noPath":    "Pfad zur Clusterball.exe nicht gesetzt.\nBitte ⚙ Einstellungen öffnen.",
	"dlg.notFound":  "Clusterball.exe nicht gefunden:\n%s",
	"dlg.heartbeat": "Heartbeat-Fehler",
	"dlg.gameCfg":   "config.cfg des Spiels konnte nicht geschrieben werden:\n%s",
	"dlg.pickExe":   "Clusterball.exe auswählen",
	"dlg.filterCB":  "Clusterball (Clusterball.exe)|Clusterball.exe|Alle Programme (*.exe)|*.exe",

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

	"set.program":    "Clusterball program",
	"set.browse":     "Browse …",
	"set.browseTip":  "Select Clusterball.exe",
	"set.graphics":   "Graphics",
	"set.renderer":   "Renderer:",
	"set.resolution": "Resolution:",
	"set.network":    "Network",
	"set.master":     "Master server IP:",
	"set.language":   "Language",
	"set.save":       "✓  Save settings",
	"set.saveTip":    "Saves to %s",

	"set.depth":      "Color depth:",
	"set.depth16":    "16 bit",
	"set.depthTip":   "Glide uses a 16-bit framebuffer – the interface offers no other colour depth. For finer textures use the \"32-bit textures\" switch instead.",
	"set.quality":    "Detail level:",
	"set.qLow":       "low",
	"set.qMid":       "medium",
	"set.qHigh":      "high",
	"set.brightness": "Brightness:",
	"set.antialias":  "Anti-aliasing",
	"set.trilinear":  "Trilinear filtering",
	"set.tex32":      "32-bit textures",
	"set.clamp":      "Clamp texture edges",
	"set.gfxHint":    "applies on next game start",

	"set.resolutionMax": "(game maximum)",
	"set.resolutionTip": "Fixed to the highest resolution Clusterball itself supports – the game only offers 640 × 480, 800 × 600 and 1024 × 768. For a sharper image, raise the wrapper's render resolution.",

	"set.glide":         "Glide wrapper (nGlide)",
	"set.glideRes":      "Render resolution:",
	"set.glideResTip":   "The resolution nGlide actually draws the 3D scene at, independent of what the game asks for. Higher means a sharper image, not upscaling.",
	"set.glideResApp":   "as in game",
	"set.glideResDsk":   "as desktop",
	"set.glideAspect":   "Aspect ratio:",
	"set.glideAspFit":   "fill screen",
	"set.glideAspApp":   "as in game",
	"set.glideAsp43":    "4:3",
	"set.glideVSync":    "Vertical synchronisation",
	"set.glideSplash":   "3dfx logo on start",
	"set.glideOnly3dfx": "only effective with the 3dfx renderer",

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
	"st.exeMissing":  "Clusterball.exe not found – please set it below",
	"st.joining":     "Starting for “%s” …",
	"st.cfgFailed":   "Could not save configuration: %s",

	"dlg.error":     "Error",
	"dlg.noPath":    "Path to Clusterball.exe is not set.\nPlease open ⚙ Settings.",
	"dlg.notFound":  "Clusterball.exe not found:\n%s",
	"dlg.heartbeat": "Heartbeat error",
	"dlg.gameCfg":   "Could not write the game's config.cfg:\n%s",
	"dlg.pickExe":   "Select Clusterball.exe",
	"dlg.filterCB":  "Clusterball (Clusterball.exe)|Clusterball.exe|All programs (*.exe)|*.exe",

	"sb.force": "Get ClusterForce!",
	"sb.brand": "clusterball.de · XDREAM GAMES NETWORK",
}
