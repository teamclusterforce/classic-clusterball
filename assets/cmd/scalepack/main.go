// scalepack -tex <textures> -list <hashes.txt> -factor 8 -cap 2048 -out <png dir>: writes each listed texture at
// factor x its original size, but no edge above cap, resampled from the smallest upscale that is large enough
// (up\ = Real-ESRGAN 4x, up16\ = 16x). Textures below 16 px have no upscale and are left out; build_scaled_pack.ps1
// compresses the result to BC7.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
)

func load(p string) (image.Image, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func pngSize(p string) (int, int, error) {
	f, err := os.Open(p)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	c, err := png.DecodeConfig(f)
	return c.Width, c.Height, err
}

func main() {
	tex := flag.String("tex", `C:\temp\cfleg\run\textures`, "texture folder with dump\\, up\\ and up16\\")
	list := flag.String("list", "", "file with one texture hash per line")
	factor := flag.Float64("factor", 8, "size relative to the original")
	capPx := flag.Int("cap", 2048, "largest edge; 0 = none")
	out := flag.String("out", "", "folder for the PNGs")
	flag.Parse()
	if *list == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	f, err := os.Open(*list)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	var hashes []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if h := strings.TrimSpace(sc.Text()); h != "" {
			hashes = append(hashes, h)
		}
	}
	f.Close()
	os.MkdirAll(*out, 0o755)
	done, small, noSource := 0, 0, 0
	for _, h := range hashes {
		w, ht, err := pngSize(filepath.Join(*tex, "dump", h+".png"))
		if err != nil {
			fmt.Println("no original:", h)
			continue
		}
		if w < 16 || ht < 16 {
			small++
			continue
		}
		s := *factor
		if e := float64(max(w, ht)); *capPx > 0 && e*s > float64(*capPx) {
			s = float64(*capPx) / e
		}
		tw, th := int(math.Round(float64(w)*s)), int(math.Round(float64(ht)*s))
		// The smallest upscale at least as large as the target; 16x only where 4x falls short.
		src := filepath.Join(*tex, "up", h+".png")
		if s > 4 {
			if _, err := os.Stat(filepath.Join(*tex, "up16", h+".png")); err == nil {
				src = filepath.Join(*tex, "up16", h+".png")
			} else {
				fmt.Println("no 16x upscale, using 4x:", h)
				noSource++
			}
		}
		img, err := load(src)
		if err != nil {
			fmt.Println(h, err)
			noSource++
			continue
		}
		var res image.Image = img
		if b := img.Bounds(); b.Dx() != tw || b.Dy() != th {
			// RGBA is premultiplied, so transparent pixels do not bleed their colour into the edges.
			dst := image.NewRGBA(image.Rect(0, 0, tw, th))
			draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
			res = dst
		}
		o, err := os.Create(filepath.Join(*out, h+".png"))
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		png.Encode(o, res)
		o.Close()
		done++
	}
	fmt.Printf("%d written, %d below 16 px left out, %d without a large enough upscale\n", done, small, noSource)
}
