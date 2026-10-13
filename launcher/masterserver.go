package main

// Netzwerk-Protokoll für Clusterball Master-Server
//
// Hostliste: HTTP GET https://www.clusterball.de/app/cb_proxy.php?q=hosts,players
// JSON-Format: [[headers, ...host_rows...], [player_headers, ...player_rows...]]
// Host-Felder: Hostname, Venue, Players, Time, Host, State, Conn, Type, mc, pw, pl
//
// Host-Registrierung: UDP-Heartbeat an 178.115.244.173:12301
// Paket-Format (38 Bytes), aus Reverse-Engineering:
//   [0] 0E 00       - Header
//   [2] 26 00       - Paketlänge (38, little-endian)
//   [4] conn_type   - Verbindungstyp: 0x00=Modem, 0x01=ISDN, 0x02=LAN
//   [5] host_type   - Host-Typ: 0x02=Temporär, 0x01=Dedicated
//   [6-9]           - Session-ID (4 Bytes)
//   [10-15]         - Flags/Nullbytes (6 Bytes)
//   [16] 05         - State (5=offen)
//   [17-20]         - Game-Parameter (4 Bytes)
//   [21] 00         - Padding
//   [22-37]         - Venue-Name, null-terminiert, 16 Bytes

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	masterServerAddr  = "178.115.244.173:12301"
	masterServerHTTP  = "https://www.clusterball.de/app/cb_proxy.php?q=hosts,players"
	heartbeatInterval = 20 * time.Second

	connModem = 0x00
	connISDN  = 0x01
	connLAN   = 0x02

	hostTypeTemp      = 0x02
	hostTypeDedicated = 0x01
)

// HostEntry repräsentiert einen Spielserver in der Hostliste.
type HostEntry struct {
	Name       string
	IP         string
	Port       uint16
	Players    int
	MaxPlayers int
	PlayersStr string // z.B. "0/8"
	Map        string
	Ping       int
	HostType   string // "D"=Dedicated, andere=Temp
	GameType   string // z.B. "FFA"
}

// MasterClient verwaltet die Master-Server-Kommunikation.
type MasterClient struct {
	serverIP string
}

// NewMasterClient erstellt einen neuen Master-Server-Client.
func NewMasterClient(serverIP string) *MasterClient {
	return &MasterClient{serverIP: serverIP}
}

// GetHostList holt die Hostliste per HTTP von clusterball.de.
func (mc *MasterClient) GetHostList() ([]HostEntry, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(masterServerHTTP)
	if err != nil {
		return nil, fmt.Errorf("HTTP-Anfrage fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Antwort lesen: %w", err)
	}
	return parseHostList(body)
}

// parseHostList wertet die Antwort von cb_proxy.php aus. Bewusst getrennt vom
// HTTP-Teil, damit das Format ohne Netzwerk und ohne laufenden Host geprüft
// werden kann (siehe masterserver_test.go).
//
// JSON-Format: [[host_header, host_row...], [player_header, player_row...]]
// Host-Spalten: Hostname, Venue, Players, Time, Host, State, Conn, Type, mc, pw, pl
func parseHostList(body []byte) ([]HostEntry, error) {
	var result [][][]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON-Parsing: %w", err)
	}

	if len(result) == 0 || len(result[0]) < 2 {
		return nil, nil // nur Kopfzeile: kein Host online
	}

	var hosts []HostEntry
	for i := 1; i < len(result[0]); i++ {
		row := result[0][i]
		if len(row) < 3 {
			continue
		}
		h := HostEntry{
			Name:       toString(row[0]),
			Map:        toString(row[1]),
			PlayersStr: toString(row[2]),
		}
		// Die Spalte "Host" führt je nach Eintrag ein Kennzeichen ("D" für
		// Dedicated) oder die Adresse des Hosts. Beides kann vorkommen, also
		// wird unterschieden statt geraten - eine Adresse hier ist das, was
		// beim Beitreten als Host-IP gebraucht wird.
		if len(row) > 4 {
			v := strings.TrimSpace(toString(row[4]))
			if net.ParseIP(v) != nil {
				h.IP = v
			} else {
				h.HostType = v
			}
		}
		if len(row) > 7 {
			h.GameType = toString(row[7])
		}
		hosts = append(hosts, h)
	}
	return hosts, nil
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func toInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

// HostBeacon verwaltet den UDP-Heartbeat für einen eigenen Dedicated Host.
type HostBeacon struct {
	mu        sync.Mutex
	conn      *net.UDPConn
	running   bool
	sessionID [4]byte
	venue     string
	hostType  byte
	connType  byte
}

// NewHostBeacon erstellt einen neuen Heartbeat-Sender.
//
// Eine Host-Adresse wird bewusst nicht übergeben: das Paket enthält kein Feld
// dafür (siehe buildPacket), der Master-Server nimmt die Adresse aus dem
// Absender des UDP-Pakets.
func NewHostBeacon(venue string, dedicated bool) *HostBeacon {
	ht := byte(hostTypeTemp)
	if dedicated {
		ht = hostTypeDedicated
	}
	var sid [4]byte
	rand.Read(sid[:])
	return &HostBeacon{
		sessionID: sid,
		venue:     venue,
		hostType:  ht,
		connType:  connLAN,
	}
}

// Start beginnt mit dem Senden von UDP-Heartbeat-Paketen.
func (b *HostBeacon) Start() error {
	addr, err := net.ResolveUDPAddr("udp4", masterServerAddr)
	if err != nil {
		return fmt.Errorf("Adresse auflösen: %w", err)
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return fmt.Errorf("UDP-Verbindung: %w", err)
	}

	b.mu.Lock()
	b.conn = conn
	b.running = true
	b.mu.Unlock()

	go b.loop()
	return nil
}

// Stop beendet den Heartbeat.
func (b *HostBeacon) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.running = false
	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
	}
}

func (b *HostBeacon) loop() {
	b.send()
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for range ticker.C {
		b.mu.Lock()
		running := b.running
		b.mu.Unlock()
		if !running {
			return
		}
		b.send()
	}
}

func (b *HostBeacon) send() {
	pkt := b.buildPacket()
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()
	if conn != nil {
		conn.Write(pkt)
	}
}

// buildPacket erstellt das 38-Byte UDP-Registrierungspaket.
// Exaktes Format aus Reverse-Engineering (Capture 2026-08-09):
// 0E 00 26 00 08 02 68 30 13 00 00 00 00 00 00 05 6A 78 CC 01 00 [venue 17B]
func (b *HostBeacon) buildPacket() []byte {
	pkt := make([]byte, 38)

	pkt[0] = 0x0E
	pkt[1] = 0x00
	pkt[2] = 0x26 // Länge = 38
	pkt[3] = 0x00
	pkt[4] = 0x08       // Verbindungs-/Paket-Flags (aus Capture)
	pkt[5] = b.hostType // 0x02 = temp/normal, 0x01 = dedicated
	copy(pkt[6:10], b.sessionID[:])
	// pkt[10..14] = 0x00 (5 Bytes Padding)
	pkt[15] = 0x05 // State: 5 = offen
	pkt[16] = 0x6A // Game-Parameter [16-19] aus Capture
	pkt[17] = 0x78
	pkt[18] = 0xCC
	pkt[19] = 0x01
	pkt[20] = 0x00 // Padding
	// Venue-Name: null-terminiert, 17 Bytes ab Position 21
	venue := b.venue
	if venue == "" {
		venue = "Egypt"
	}
	venueBytes := []byte(strings.ToUpper(venue))
	if len(venueBytes) > 16 {
		venueBytes = venueBytes[:16]
	}
	copy(pkt[21:], venueBytes)
	// Rest bleibt 0

	return pkt
}
