package main

import (
	"strings"
	"testing"
)

// Beide Sprachen haben dieselben Kapitel mit demselben Aufbau, sonst springt
// der Hilfe-Reiter beim Umschalten auf ein anderes Kapitel oder es fehlt Text.
func TestHilfeBeideSprachenGleichAufgebaut(t *testing.T) {
	gleichAufgebaut(t, helpDE, helpEN, 10)
	gleichAufgebaut(t, aboutDE, aboutEN, 1)
}

func gleichAufgebaut(t *testing.T, srcDE, srcEN string, minChapters int) {
	de, en := parseHelp(srcDE), parseHelp(srcEN)
	if len(de) < minChapters || len(de) != len(en) {
		t.Fatalf("Kapitel: de %d, en %d", len(de), len(en))
	}
	for i := range de {
		if len(de[i].blocks) != len(en[i].blocks) {
			t.Errorf("Kapitel %q/%q: %d gegen %d Bloecke", de[i].title, en[i].title, len(de[i].blocks), len(en[i].blocks))
			continue
		}
		for j := range de[i].blocks {
			a, b := de[i].blocks[j], en[i].blocks[j]
			if a.kind != b.kind || a.ball != b.ball || a.width != b.width {
				t.Errorf("Kapitel %q, Block %d: %q gegen %q", de[i].title, j, a.term+a.text, b.term+b.text)
			}
		}
	}
}

func TestHilfeBloecke(t *testing.T) {
	for i, src := range []string{helpDE, helpEN, aboutDE, aboutEN} {
		for _, ch := range parseHelp(src) {
			if ch.title == "" || len(ch.blocks) == 0 {
				t.Errorf("leeres Kapitel %q", ch.title)
			}
			for _, b := range ch.blocks {
				switch b.kind {
				case hbImage:
					if _, ok := helpImages[b.text]; !ok {
						t.Errorf("Bild %q nicht eingebettet", b.text)
					}
				case hbLink:
					if !strings.HasPrefix(b.url, "https://") || b.text == "" {
						t.Errorf("Link %q -> %q", b.text, b.url)
					}
				case hbTerm:
					if b.term == "" || b.text == "" {
						t.Errorf("Begriff ohne Text: %q", b.term+b.text)
					}
				}
				// Kein Rest der alten Installations- und XDCC-Kapitel (nur Hilfe;
				// "Über" nennt Glide als das, was ersetzt wurde).
				if i >= 2 {
					continue
				}
				for _, bad := range []string{"XDCC", "XDream", "Glide", "3dfx"} {
					if strings.Contains(b.term+b.text, bad) {
						t.Errorf("%q in %q", bad, b.term+b.text)
					}
				}
			}
		}
	}
}

func TestHilfeFormat(t *testing.T) {
	ch := parseHelp("# Kommentar\n= Eins\n## Unter\nAbsatz\n- Punkt\n@r ROT :: 1 Punkt\nA :: B\n[img x.jpg]\n[img y.png|220]\n[link Text|https://a.b/]\n")
	if len(ch) != 1 || ch[0].title != "Eins" {
		t.Fatalf("%+v", ch)
	}
	want := []helpBlock{
		{kind: hbSub, text: "Unter"},
		{kind: hbPara, text: "Absatz"},
		{kind: hbBullet, text: "Punkt"},
		{kind: hbTerm, term: "ROT", text: "1 Punkt", ball: 'r'},
		{kind: hbTerm, term: "A", text: "B"},
		{kind: hbImage, text: "x.jpg"},
		{kind: hbImage, text: "y.png", width: 220},
		{kind: hbLink, text: "Text", url: "https://a.b/"},
	}
	if len(ch[0].blocks) != len(want) {
		t.Fatalf("%d Bloecke: %+v", len(ch[0].blocks), ch[0].blocks)
	}
	for i, w := range want {
		if ch[0].blocks[i] != w {
			t.Errorf("Block %d: %+v, erwartet %+v", i, ch[0].blocks[i], w)
		}
	}
}

func TestSpracheAusLANGID(t *testing.T) {
	for id, want := range map[uint16]string{0x0407: "de", 0x0c07: "de", 0x0807: "de", 0x0409: "en", 0x040c: "en", 0x0415: "en"} {
		if got := langOfLANGID(id); got != want {
			t.Errorf("LANGID %#04x: %q, erwartet %q", id, got, want)
		}
	}
}
