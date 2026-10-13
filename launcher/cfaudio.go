// Musik und Ball-Sound von ClusterForce statt der des Spiels (Einstellungen →
// Ton). Die Dateien erzeugt assets\build_cf_audio.ps1 aus den ClusterForce-
// Mastern: je Venue sowie fuer das Outro ein 30-Sekunden-Ausschnitt, als Intro der
// ganze Song des ClusterForce-Teasers, alles als MP3
// mit 128 kbit/s, dazu die Ball-Sounds als WAV im Format des Spiels.
//
// Die Originaldateien des Spiels bleiben unangetastet. Der Launcher legt beim
// Spielstart alles unter clusterforce\ im Spielordner ab und traegt in der
// clusterball_d3d.ini ein, was der Renderer statt des Originals nehmen soll:
// MusicDir (die Musik spielt das Spiel ueber BASS aus Levels\<Venue>.mp3 und
// MUSIC\Intro.mp3/Outro.mp3) und DataArchive, eine Kopie der Data.mrg, in der
// nur die Ball-Sounds ersetzt sind (sie stecken im Archiv): Ball.wav ist der
// Sprecher, der im Spiel "Ball" sagt; Ball1-3.wav sind die dumpfen "Blobs" des
// alten Blob-Modus ("Blob statt Ballmann"), die mit ersetzt werden.

package main

import (
	"bytes"
	"embed"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

//go:embed audio/music/*.mp3 audio/ball/*.wav
var cfAudio embed.FS

const (
	cfAudioDir    = "clusterforce"
	cfMusicSubdir = `clusterforce\music`
)

// Ball-Sound-Varianten: Wert in den Einstellungen -> WAV fuer Ball.wav, Ball1,
// Ball2, Ball3. "" ist der Sound des Spiels. Die Sprecher-Ansage gibt es in
// ClusterForce von zwei Sprechern; der erste spricht "Ball", in den Blob-
// Plaetzen wechseln sie sich ab.
var ballSoundSets = map[string][4]string{
	"standard":  {"standard", "standard", "standard", "standard"},
	"computer":  {"computer", "computer", "computer", "computer"},
	"announcer": {"announcer1", "announcer1", "announcer2", "announcer1"},
}

// ballSlots: die Eintraege der Data.mrg in der Reihenfolge von ballSoundSets.
var ballSlots = [4]string{"Ball.wav", "Ball1.wav", "Ball2.wav", "Ball3.wav"}

// ballSoundValues: Reihenfolge der Auswahlliste.
var ballSoundValues = []string{"", "standard", "computer", "announcer"}

func ballSoundLabels() []string {
	return []string{tr("snd.ballOriginal"), tr("snd.ballStandard"), tr("snd.ballComputer"), tr("snd.ballAnnouncer")}
}

// dataArchiveName ist die Kopie der Data.mrg fuer eine Ball-Sound-Variante.
func dataArchiveName(variant string) string {
	return filepath.Join(cfAudioDir, "Data.ball-"+variant+".mrg")
}

// ballVariantOf liest die Variante aus dem DataArchive-Eintrag der INI zurueck.
func ballVariantOf(archive string) string {
	b := strings.ToLower(filepath.Base(archive))
	if !strings.HasPrefix(b, "data.ball-") || !strings.HasSuffix(b, ".mrg") {
		return ""
	}
	v := strings.TrimSuffix(strings.TrimPrefix(b, "data.ball-"), ".mrg")
	if _, ok := ballSoundSets[v]; !ok {
		return ""
	}
	return v
}

// installCFAudio legt Musik und Ball-Sound-Archiv der gewaehlten Einstellungen
// im Spielordner ab. Vorhandene Dateien gleicher Groesse bleiben stehen.
func installCFAudio(dir string, d *D3DSettings) error {
	if d.CFMusic {
		out := filepath.Join(dir, cfMusicSubdir)
		if err := os.MkdirAll(out, 0755); err != nil {
			return err
		}
		files, _ := cfAudio.ReadDir("audio/music")
		for _, f := range files {
			data, err := cfAudio.ReadFile("audio/music/" + f.Name())
			if err != nil {
				return err
			}
			if err := writeIfDifferent(filepath.Join(out, f.Name()), data); err != nil {
				return err
			}
		}
	}
	if d.BallSound != "" {
		set, ok := ballSoundSets[d.BallSound]
		if !ok {
			return nil
		}
		orig, err := os.ReadFile(filepath.Join(dir, "Data.mrg"))
		if err != nil {
			return err
		}
		repl := map[string][]byte{}
		for i, name := range set {
			wav, err := cfAudio.ReadFile("audio/ball/" + name + ".wav")
			if err != nil {
				return err
			}
			repl[ballSlots[i]] = wav
		}
		patched, err := patchArchive(orig, repl)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(dir, cfAudioDir), 0755); err != nil {
			return err
		}
		if err := writeIfDifferent(filepath.Join(dir, dataArchiveName(d.BallSound)), patched); err != nil {
			return err
		}
	}
	return nil
}

func writeIfDifferent(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// patchArchive gibt eine Kopie eines Clusterball-Archivs (Data.mrg) zurueck,
// in der die Eintraege aus repl (Dateiname ohne die vorangestellte Nummer,
// z.B. "Ball1.wav") ersetzt sind. Die neuen Daten werden angehaengt und Offset
// und Laenge im Verzeichnis umgestellt; alles andere bleibt Byte fuer Byte.
//
// Aufbau (big-endian): "???", Anzahl der Gruppen, dann je Gruppe: Blocklaenge,
// Name (NUL-terminiert), Anzahl der Eintraege, je Eintrag: id, Offset, Laenge,
// Name ("31.shipLOD358.3GM", NUL-terminiert).
func patchArchive(orig []byte, repl map[string][]byte) ([]byte, error) {
	be := binary.BigEndian
	out := append([]byte(nil), orig...)
	cstr := func(p int) (string, int, error) {
		e := bytes.IndexByte(orig[p:], 0)
		if e < 0 {
			return "", 0, errors.New("mrg: Name ohne Ende")
		}
		return string(orig[p : p+e]), p + e + 1, nil
	}
	if len(orig) < 8 {
		return nil, errors.New("mrg: zu kurz")
	}
	found := map[string]bool{}
	nf := int(be.Uint32(orig[4:]))
	p := 8
	for i := 0; i < nf; i++ {
		if p+4 > len(orig) {
			return nil, errors.New("mrg: Verzeichnis laeuft ueber das Ende")
		}
		bl := int(be.Uint32(orig[p:]))
		_, q, err := cstr(p + 4)
		if err != nil {
			return nil, err
		}
		n := int(be.Uint32(orig[q:]))
		q += 4
		for j := 0; j < n; j++ {
			if q+12 > len(orig) {
				return nil, errors.New("mrg: Eintrag laeuft ueber das Ende")
			}
			name, nq, err := cstr(q + 12)
			if err != nil {
				return nil, err
			}
			short := name
			if k := strings.IndexByte(name, '.'); k >= 0 {
				short = name[k+1:]
			}
			for key, data := range repl {
				if strings.EqualFold(short, key) {
					for len(out)%4 != 0 {
						out = append(out, 0)
					}
					be.PutUint32(out[q+4:], uint32(len(out)))
					be.PutUint32(out[q+8:], uint32(len(data)))
					out = append(out, data...)
					found[key] = true
				}
			}
			q = nq
		}
		if bl <= 0 {
			return nil, errors.New("mrg: leerer Block")
		}
		p += bl
	}
	for key := range repl {
		if !found[key] {
			return nil, errors.New("mrg: " + key + " nicht gefunden")
		}
	}
	return out, nil
}

func musicLabels() []string { return []string{tr("snd.musicOriginal"), tr("snd.musicCF")} }

var (
	procPlaySound = syscall.NewLazyDLL("winmm.dll").NewProc("PlaySoundW")
	previewWav    []byte // muss leben, solange PlaySound asynchron spielt
)

// previewBallSound spielt den in der Liste gewaehlten Ball-Sound einmal ab -
// beim Original den Sprecher (Ball.wav) aus der Data.mrg des Spiels.
func (mw *AppWindow) previewBallSound() {
	i := mw.cmbBall.CurrentIndex()
	if i < 0 || i >= len(ballSoundValues) {
		return
	}
	var wav []byte
	if v := ballSoundValues[i]; v == "" {
		data, err := os.ReadFile(filepath.Join(cbExeDir(&mw.cfg), "Data.mrg"))
		if err != nil {
			mw.setStatus(err.Error())
			return
		}
		if wav, err = mrgEntry(data, "Ball.wav"); err != nil {
			mw.setStatus(err.Error())
			return
		}
		wav = append([]byte(nil), wav...)
	} else {
		var err error
		if wav, err = cfAudio.ReadFile("audio/ball/" + ballSoundSets[v][0] + ".wav"); err != nil {
			return
		}
	}
	const sndAsync, sndNoDefault, sndMemory = 0x0001, 0x0002, 0x0004
	previewWav = wav
	procPlaySound.Call(uintptr(unsafe.Pointer(&previewWav[0])), 0, sndAsync|sndNoDefault|sndMemory)
}
