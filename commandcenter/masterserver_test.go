package main

import "testing"

// Kopfzeile wie sie cb_proxy.php tatsächlich liefert.
const hostHeader = `["Hostname","Venue","Players","Time","Host","State","Conn","Type","mc","pw","pl"]`
const playerBlock = `[["Country","Flag","Player","Rank","Title","Last contact"]]`

func TestParseHostListLeer(t *testing.T) {
	// Genau die Antwort, die der Master-Server liefert, wenn niemand hostet.
	body := []byte(`[[` + hostHeader + `],` + playerBlock + `]`)

	hosts, err := parseHostList(body)
	if err != nil {
		t.Fatalf("unerwarteter Fehler: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("erwartet 0 Hosts, bekommen %d", len(hosts))
	}
}

func TestParseHostListEintraege(t *testing.T) {
	body := []byte(`[[` + hostHeader + `,
		["Nigels Arena","Egypt","3/8","12:04","D","5","2","FFA",0,0,0],
		["LAN Runde","Antarctica","1/8","00:31","92.72.169.107","5","2","Team",0,0,0]
	],` + playerBlock + `]`)

	hosts, err := parseHostList(body)
	if err != nil {
		t.Fatalf("unerwarteter Fehler: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("erwartet 2 Hosts, bekommen %d", len(hosts))
	}

	h := hosts[0]
	if h.Name != "Nigels Arena" || h.Map != "Egypt" || h.PlayersStr != "3/8" || h.GameType != "FFA" {
		t.Errorf("Zeile 1 falsch gelesen: %+v", h)
	}
	if h.HostType != "D" {
		t.Errorf("Zeile 1: HostType = %q, erwartet \"D\"", h.HostType)
	}
	if h.IP != "" {
		t.Errorf("Zeile 1: IP = %q, erwartet leer", h.IP)
	}

	// Zweite Zeile führt in derselben Spalte eine Adresse statt des Kennzeichens.
	h = hosts[1]
	if h.IP != "92.72.169.107" {
		t.Errorf("Zeile 2: IP = %q, erwartet 92.72.169.107", h.IP)
	}
	if h.HostType != "" {
		t.Errorf("Zeile 2: HostType = %q, erwartet leer", h.HostType)
	}
}

// Die Anzeige der Typ-Spalte hängt an HostType/GameType - hier abgesichert,
// weil das Modell die einzige Stelle ist, die beides zusammenführt.
func TestHostListModelTypSpalte(t *testing.T) {
	m := NewHostListModel()
	m.setHosts([]HostEntry{
		{Name: "A", HostType: "D", GameType: "FFA"},
		{Name: "B", HostType: "D"},
		{Name: "C", GameType: "Team"},
	})

	if m.RowCount() != 3 {
		t.Fatalf("RowCount = %d, erwartet 3", m.RowCount())
	}
	if got := m.Value(0, 3); got != "Dedi · FFA" {
		t.Errorf("Zeile 0 Typ = %q", got)
	}
	if got := m.Value(1, 3); got != tr("srv.dedicated") {
		t.Errorf("Zeile 1 Typ = %q", got)
	}
	if got := m.Value(2, 3); got != "Team" {
		t.Errorf("Zeile 2 Typ = %q", got)
	}
	// Ausserhalb des Bereichs darf nicht panicken - die TableView fragt
	// waehrend eines Refresh gelegentlich alte Zeilennummern ab.
	if got := m.Value(99, 0); got != "" {
		t.Errorf("Zeile 99 = %q, erwartet leer", got)
	}
}
