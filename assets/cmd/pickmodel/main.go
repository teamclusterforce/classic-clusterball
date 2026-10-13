// pickmodel: for every original sprite, compares two 4x upscales (A, B) after box-filtering them back to the
// original size and prints which one to keep. A is preferred (it draws cleaner edges); B is taken when A
// deviates from the original by more than -factor times B's deviation, i.e. A invented shapes (garbled text).
//
//	pickmodel -orig in -a up_anime -b up_x4plus -factor 1.6 > choice.txt   (lines "<file> A|B errA errB")
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
)

func load(fn string) image.Image {
	f, err := os.Open(fn)
	if err != nil {
		return nil
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil
	}
	return img
}

// deviation: mean over original pixels of |down(up) - orig| in premultiplied RGBA, 0..255
func deviation(orig, up image.Image) float64 {
	ob, ub := orig.Bounds(), up.Bounds()
	k := ub.Dx() / ob.Dx()
	if k < 1 || ub.Dy()/ob.Dy() != k {
		return 1e9
	}
	sum := 0.0
	for y := 0; y < ob.Dy(); y++ {
		for x := 0; x < ob.Dx(); x++ {
			var s [4]float64
			for yy := 0; yy < k; yy++ {
				for xx := 0; xx < k; xx++ {
					r, g, b, a := up.At(ub.Min.X+x*k+xx, ub.Min.Y+y*k+yy).RGBA()
					s[0] += float64(r)
					s[1] += float64(g)
					s[2] += float64(b)
					s[3] += float64(a)
				}
			}
			r, g, b, a := orig.At(ob.Min.X+x, ob.Min.Y+y).RGBA()
			o := [4]float64{float64(r), float64(g), float64(b), float64(a)}
			for c := 0; c < 4; c++ {
				d := s[c]/float64(k*k) - o[c]
				if d < 0 {
					d = -d
				}
				sum += d / 257
			}
		}
	}
	return sum / float64(4*ob.Dx()*ob.Dy())
}

func main() {
	origDir := flag.String("orig", "", "originals")
	aDir := flag.String("a", "", "preferred upscales")
	bDir := flag.String("b", "", "fallback upscales")
	factor := flag.Float64("factor", 1.6, "take B when errA > factor * errB")
	flag.Parse()
	files, _ := filepath.Glob(filepath.Join(*origDir, "*.png"))
	for _, fn := range files {
		base := filepath.Base(fn)
		o, a, b := load(fn), load(filepath.Join(*aDir, base)), load(filepath.Join(*bDir, base))
		if o == nil || a == nil || b == nil {
			continue
		}
		ea, eb := deviation(o, a), deviation(o, b)
		pick := "A"
		if ea > *factor*eb {
			pick = "B"
		}
		fmt.Printf("%s %s %.2f %.2f\n", base, pick, ea, eb)
	}
}
