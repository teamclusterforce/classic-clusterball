package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mrg baut ein kleines Archiv im Format der Data.mrg: eine Gruppe mit den
// Eintraegen in der angegebenen Reihenfolge.
func mrg(entries ...[2]string) []byte {
	be := binary.BigEndian
	var dir bytes.Buffer
	dir.WriteString("sounds\x00")
	cnt := make([]byte, 4)
	be.PutUint32(cnt, uint32(len(entries)))
	dir.Write(cnt)
	// Verzeichnisgroesse vorab, damit die Offsets stimmen
	size := 8 + 4 + dir.Len()
	for i, e := range entries {
		size += 12 + len(e[0]) + 1
		_ = i
	}
	off := size
	var data bytes.Buffer
	for i, e := range entries {
		rec := make([]byte, 12)
		be.PutUint32(rec[0:], uint32(i+1))
		be.PutUint32(rec[4:], uint32(off+data.Len()))
		be.PutUint32(rec[8:], uint32(len(e[1])))
		dir.Write(rec)
		dir.WriteString(e[0] + "\x00")
		data.WriteString(e[1])
	}
	out := make([]byte, 8)
	copy(out, "MRG\x00")
	be.PutUint32(out[4:], 1)
	bl := make([]byte, 4)
	be.PutUint32(bl, uint32(4+dir.Len()))
	out = append(out, bl...)
	out = append(out, dir.Bytes()...)
	return append(out, data.Bytes()...)
}

func TestPatchArchive(t *testing.T) {
	orig := mrg([2]string{"14.Ball.wav", "ball"}, [2]string{"100.Ball1.wav", "one"}, [2]string{"101.Ball2.wav", "two"})
	got, err := patchArchive(orig, map[string][]byte{"Ball2.wav": []byte("NEU-ZWEI")})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"Ball.wav": "ball", "Ball1.wav": "one", "Ball2.wav": "NEU-ZWEI"} {
		d, err := mrgEntry(got, name)
		if err != nil || string(d) != want {
			t.Errorf("%s: %q (%v), erwartet %q", name, d, err, want)
		}
	}
	// Der Originalteil bleibt bis auf Offset/Laenge des Eintrags gleich, die
	// alten Daten bleiben stehen.
	if !bytes.Contains(got, []byte("ballonetwo")) {
		t.Error("alte Daten fehlen")
	}
	if _, err := patchArchive(orig, map[string][]byte{"Ball9.wav": nil}); err == nil {
		t.Error("fehlender Eintrag nicht gemeldet")
	}
}

// Mit der echten Data.mrg des Spiels, sofern vorhanden.
func TestPatchArchiveEchteDataMrg(t *testing.T) {
	data, err := os.ReadFile(`C:\clusterball\Data.mrg`)
	if err != nil {
		t.Skip("keine Data.mrg")
	}
	set := ballSoundSets["announcer"]
	repl := map[string][]byte{}
	for i, n := range set {
		w, err := cfAudio.ReadFile("audio/ball/" + n + ".wav")
		if err != nil {
			t.Fatal(err)
		}
		repl[ballSlots[i]] = w
	}
	got, err := patchArchive(data, repl)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range repl {
		d, err := mrgEntry(got, name)
		if err != nil || !bytes.Equal(d, want) {
			t.Errorf("%s nicht ersetzt", name)
		}
	}
	// alle anderen Eintraege unveraendert lesbar
	for _, n := range []string{"PickBall.wav", "InkBall.wav", "shipLOD358.3GM"} {
		a, _ := mrgEntry(data, n)
		b, _ := mrgEntry(got, n)
		if !bytes.Equal(a, b) {
			t.Errorf("%s veraendert", n)
		}
	}
}

// Jede Venue des Spiels sowie Intro und Outro haben einen Track, jeder Ball-
// Sound liegt im Format der Ansagen des Spiels vor (Ball.wav: 22050 Hz Mono,
// 16 Bit, schlichter 44-Byte-Kopf).
func TestEingebetteterTon(t *testing.T) {
	for _, n := range []string{"Intro", "Outro", "Antarctica", "Bora Bora", "China", "Easter Island", "Egal", "Egypt",
		"Electronica", "Gobi", "Green Garden V2", "Hangarball", "HelterSkelter", "Lunar", "Matterhorn", "Metropolis",
		"Pole Position", "Ruhrmansk", "ShootLoop", "Stonehenge", "Taj Mahal", "Tuanake", "Tutorial", "Yucatan"} {
		d, err := cfAudio.ReadFile("audio/music/" + n + ".mp3")
		if err != nil || len(d) < 100000 {
			t.Errorf("Musik %s fehlt oder ist zu klein", n)
		}
	}
	le := binary.LittleEndian
	for _, set := range ballSoundSets {
		for _, n := range set {
			d, err := cfAudio.ReadFile("audio/ball/" + n + ".wav")
			if err != nil {
				t.Fatalf("%s: %v", n, err)
			}
			if string(d[0:4]) != "RIFF" || string(d[8:12]) != "WAVE" || le.Uint16(d[20:]) != 1 ||
				le.Uint16(d[22:]) != 1 || le.Uint32(d[24:]) != 22050 || le.Uint16(d[34:]) != 16 || string(d[36:40]) != "data" {
				t.Errorf("%s: nicht im Format der Ansagen", n)
			}
		}
	}
}

func TestTonInDerIni(t *testing.T) {
	for _, d := range []D3DSettings{{CFMusic: true, BallSound: "announcer"}, {CFMusic: false, BallSound: ""},
		{CFMusic: true, BallSound: "computer"}} {
		ini := ParseIni("")
		applyD3DSettings(ini, &d)
		var back D3DSettings
		if v, _ := ini.Get("MusicDir"); (strings.TrimSpace(v) != "") != d.CFMusic {
			t.Errorf("MusicDir %q fuer %+v", v, d)
		}
		v, _ := ini.Get("DataArchive")
		back.BallSound = ballVariantOf(strings.TrimSpace(v))
		if back.BallSound != d.BallSound {
			t.Errorf("DataArchive %q -> %q, erwartet %q", v, back.BallSound, d.BallSound)
		}
	}
	if ballVariantOf(filepath.Join("clusterforce", "Data.ball-unbekannt.mrg")) != "" {
		t.Error("unbekannte Variante akzeptiert")
	}
}

// Die Musik landet unter Music\CF 2026, ein frueherer clusterforce\music
// verschwindet; die INI zeigt auf den neuen Ort.
func TestMusikOrt(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, cfMusicOldSubdir)
	if err := os.MkdirAll(old, 0755); err != nil {
		t.Fatal(err)
	}
	if err := installCFAudio(dir, &D3DSettings{CFMusic: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Music", "CF 2026", "Intro.mp3")); err != nil {
		t.Errorf("Intro fehlt: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("alter Musikordner noch da")
	}
	ini := ParseIni("")
	applyD3DSettings(ini, &D3DSettings{CFMusic: true})
	if v, _ := ini.Get("MusicDir"); v != `Music\CF 2026` {
		t.Errorf("MusicDir = %q", v)
	}
}

// Ohne Wahl laeuft die Musik von ClusterForce und der Ball-Sound des Spiels;
// wer "Original" waehlt, behaelt es auch nach dem naechsten Einlesen.
func TestTonVorgabe(t *testing.T) {
	dir := t.TempDir()
	cfg := AppConfig{CBExePath: filepath.Join(dir, "Clusterball.exe")}
	readD3DSettings(&cfg)
	if !cfg.D3D.CFMusic || cfg.D3D.BallSound != "" {
		t.Errorf("Vorgabe: Musik CF %v, Ball %q", cfg.D3D.CFMusic, cfg.D3D.BallSound)
	}
	cfg.D3D.CFMusic = false
	if err := writeD3DSettings(&cfg); err != nil {
		t.Fatal(err)
	}
	cfg.D3D.CFMusic = true
	readD3DSettings(&cfg)
	if cfg.D3D.CFMusic {
		t.Error("bewusst gewaehltes Original nicht behalten")
	}
}
