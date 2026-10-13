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
		repl["Ball"+string(rune('1'+i))+".wav"] = w
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
	for _, n := range []string{"Ball.wav", "PickBall.wav", "shipLOD358.3GM"} {
		a, _ := mrgEntry(data, n)
		b, _ := mrgEntry(got, n)
		if !bytes.Equal(a, b) {
			t.Errorf("%s veraendert", n)
		}
	}
}

// Jede Venue des Spiels sowie Intro und Outro haben einen Track, jeder Ball-
// Sound liegt als 44,1-kHz-Stereo-PCM vor wie Ball1-3.wav des Spiels.
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
				le.Uint16(d[22:]) != 2 || le.Uint32(d[24:]) != 44100 || le.Uint16(d[34:]) != 16 {
				t.Errorf("%s: kein 44,1-kHz-Stereo-PCM", n)
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
