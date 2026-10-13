// icon -game <game folder> -out <prefix> [-c appicon.c]: draws the Clusterball emblem as an application icon in the style of the
// original cb_gold_xdgn.ico (blue body with a bevel, white mark), from the shape in the 16x upscale of the loading
// box, and writes <prefix>.png (1024 px master) and <prefix>.ico (256 ... 16 px).
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
)

// The emblem in the loading box's 16x upscale (stored bottom-up): this rectangle, flipped.
var emblemRect = image.Rect(2330, 1040, 3370, 2020)

const S = 1024 // master size

type field []float64

func (f field) at(x, y int) float64 {
	x = min(max(x, 0), S-1)
	y = min(max(y, 0), S-1)
	return f[y*S+x]
}

// blur is a box blur of radius r, three passes (close to a Gaussian).
func blur(f field, r int) field {
	cur := append(field(nil), f...)
	tmp := make(field, len(f))
	for pass := 0; pass < 3; pass++ {
		for y := 0; y < S; y++ {
			s := 0.0
			for x := -r; x <= r; x++ {
				s += cur.at(x, y)
			}
			for x := 0; x < S; x++ {
				tmp[y*S+x] = s / float64(2*r+1)
				s += cur.at(x+r+1, y) - cur.at(x-r, y)
			}
		}
		for x := 0; x < S; x++ {
			s := 0.0
			for y := -r; y <= r; y++ {
				s += tmp.at(x, y)
			}
			for y := 0; y < S; y++ {
				cur[y*S+x] = s / float64(2*r+1)
				s += tmp.at(x, y+r+1) - tmp.at(x, y-r)
			}
		}
	}
	return cur
}

func clamp01(v float64) float64 { return math.Min(1, math.Max(0, v)) }
func smooth(a, b, v float64) float64 {
	t := clamp01((v - a) / (b - a))
	return t * t * (3 - 2*t)
}

// masks returns the coverage of the whole emblem (body and mark) and of the mark alone.
func masks(src image.Image, fill float64) (shape, mark field) {
	b := src.Bounds()
	// Fit the emblem into the master with a margin, keeping its aspect.
	sc := fill * S / float64(max(b.Dx(), b.Dy()))
	w, h := int(float64(b.Dx())*sc), int(float64(b.Dy())*sc)
	ox, oy := (S-w)/2, (S-h)/2-S/50
	fit := image.NewRGBA(image.Rect(0, 0, S, S))
	draw.Draw(fit, fit.Bounds(), image.NewUniform(color.RGBA{255, 200, 40, 255}), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(fit, image.Rect(ox, oy, ox+w, oy+h), src, b, draw.Src, nil)
	dark := make(field, S*S)
	for y := 0; y < S; y++ {
		for x := 0; x < S; x++ {
			c := fit.RGBAAt(x, y)
			l := (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
			dark[y*S+x] = smooth(0.25, 0.65, 1-l) // 0 = yellow, 1 = black
		}
	}
	// Outside: everything not dark that is connected to the border.
	out := make([]bool, S*S)
	stack := []int{}
	for i := 0; i < S; i++ {
		stack = append(stack, i, (S-1)*S+i, i*S, i*S+S-1)
	}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if out[p] || dark[p] > 0.5 {
			continue
		}
		out[p] = true
		x, y := p%S, p/S
		if x > 0 {
			stack = append(stack, p-1)
		}
		if x < S-1 {
			stack = append(stack, p+1)
		}
		if y > 0 {
			stack = append(stack, p-S)
		}
		if y < S-1 {
			stack = append(stack, p+S)
		}
	}
	shape, mark = make(field, S*S), make(field, S*S)
	for i := range dark {
		if out[i] {
			shape[i] = dark[i]
		} else {
			shape[i] = 1
			mark[i] = 1 - dark[i]
		}
	}
	return shape, mark
}

func lerp(a, b color.RGBA, t float64) [3]float64 {
	return [3]float64{float64(a.R) + (float64(b.R)-float64(a.R))*t, float64(a.G) + (float64(b.G)-float64(a.G))*t, float64(a.B) + (float64(b.B)-float64(a.B))*t}
}

// small: the variant for 48 px and below, without the drop shadow and with a darker body, so the white mark keeps
// its contrast once only a few pixels are left.
func render(shape, mark field, small bool) *image.NRGBA {
	bs := blur(shape, 14) // body relief
	bm := blur(mark, 7)   // mark relief
	sh := blur(shape, 20) // drop shadow
	top, bot := color.RGBA{86, 132, 255, 255}, color.RGBA{14, 38, 176, 255}
	if small {
		top, bot = color.RGBA{50, 96, 240, 255}, color.RGBA{8, 28, 150, 255}
	}
	img := image.NewNRGBA(image.Rect(0, 0, S, S))
	lx, ly := -0.6, -0.8 // light from the upper left
	for y := 0; y < S; y++ {
		for x := 0; x < S; x++ {
			i := y*S + x
			// Body: vertical gradient, lit along the bevel.
			c := lerp(top, bot, float64(y)/S)
			gx := (bs.at(x+2, y) - bs.at(x-2, y)) * 6
			gy := (bs.at(x, y+2) - bs.at(x, y-2)) * 6
			lit := -(gx*lx + gy*ly) // edge facing the light: positive
			for k := range c {
				if lit > 0 {
					c[k] += (255 - c[k]) * clamp01(lit) * 0.75
				} else {
					c[k] *= 1 + math.Max(lit, -1)*0.55
				}
			}
			// Gloss over the upper part.
			if g := smooth(0.62, 0.30, float64(y)/S) * 0.22; g > 0 {
				for k := range c {
					c[k] += (255 - c[k]) * g
				}
			}
			// Mark: white, shaded blue on the side away from the light.
			if m := mark[i]; m > 0 {
				mx := (bm.at(x+2, y) - bm.at(x-2, y)) * 6
				my := (bm.at(x, y+2) - bm.at(x, y-2)) * 6
				ml := -(mx*lx + my*ly)
				w := [3]float64{250, 252, 255}
				if ml < 0 {
					t := clamp01(-ml) * 0.7
					w = [3]float64{250 + (120-250)*t, 252 + (160-252)*t, 255 + (245-255)*t}
				}
				for k := range c {
					c[k] = c[k]*(1-m) + w[k]*m
				}
			}
			a := shape[i]
			// Drop shadow below the emblem.
			sa := sh.at(x, y-S/55) * 0.45
			if small {
				sa = 0
			}
			oa := a + sa*(1-a)
			if oa <= 0 {
				continue
			}
			for k := range c {
				c[k] = c[k] * a / oa // shadow colour is black
			}
			img.SetNRGBA(x, y, color.NRGBA{uint8(clamp01(c[0]/255) * 255), uint8(clamp01(c[1]/255) * 255), uint8(clamp01(c[2]/255) * 255), uint8(clamp01(oa) * 255)})
		}
	}
	return img
}

func scaled(src *image.NRGBA, n int) *image.NRGBA {
	// Premultiplied resampling, so the transparent surroundings do not darken the edges.
	pm := image.NewRGBA(src.Bounds())
	draw.Draw(pm, pm.Bounds(), src, image.Point{}, draw.Src)
	dst := image.NewRGBA(image.Rect(0, 0, n, n))
	draw.CatmullRom.Scale(dst, dst.Bounds(), pm, pm.Bounds(), draw.Src, nil)
	out := image.NewNRGBA(dst.Bounds())
	draw.Draw(out, out.Bounds(), dst, image.Point{}, draw.Src)
	return out
}

func writeICO(path string, imgs []*image.NRGBA) error {
	var dir, data bytes.Buffer
	binary.Write(&dir, binary.LittleEndian, [3]uint16{0, 1, uint16(len(imgs))})
	off := 6 + 16*len(imgs)
	for _, im := range imgs {
		var p bytes.Buffer
		png.Encode(&p, im)
		n := im.Bounds().Dx()
		b := byte(n)
		if n >= 256 {
			b = 0
		}
		dir.Write([]byte{b, b, 0, 0})
		binary.Write(&dir, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&dir, binary.LittleEndian, [2]uint32{uint32(p.Len()), uint32(off)})
		off += p.Len()
		data.Write(p.Bytes())
	}
	return os.WriteFile(path, append(dir.Bytes(), data.Bytes()...), 0o644)
}

func main() {
	game := flag.String("game", `C:\temp\cfleg\run`, "game folder with textures\\up16")
	out := flag.String("out", "clusterball", "output prefix")
	cfile := flag.String("c", "", "also write the sizes as a C source for the renderer (modern-renderer/appicon.c)")
	flag.Parse()
	f, err := os.Open(filepath.Join(*game, "textures", "up16", "ab219e4e8b4f5492.png"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	box, err := png.Decode(f)
	f.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	// Cut the emblem out and turn it upright.
	src := image.NewRGBA(image.Rect(0, 0, emblemRect.Dx(), emblemRect.Dy()))
	for y := 0; y < emblemRect.Dy(); y++ {
		for x := 0; x < emblemRect.Dx(); x++ {
			src.Set(x, emblemRect.Dy()-1-y, box.At(emblemRect.Min.X+x, emblemRect.Min.Y+y))
		}
	}
	shape, mark := masks(src, 0.86)
	master := render(shape, mark, false)
	shape, mark = masks(src, 0.98)
	small := render(shape, mark, true)
	pf, _ := os.Create(*out + ".png")
	png.Encode(pf, master)
	pf.Close()
	var imgs []*image.NRGBA
	for _, n := range []int{256, 128, 64, 48, 32, 24, 16} {
		m := master
		if n <= 48 {
			m = small
		}
		imgs = append(imgs, scaled(m, n))
	}
	if err := writeICO(*out+".ico", imgs); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("wrote", *out+".png", *out+".ico")
	if *cfile != "" {
		if err := writeC(*cfile, imgs); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Println("wrote", *cfile)
	}
}

// writeC writes the icon sizes as PNG byte arrays for the renderer, which gives the game window this icon
// (modern-renderer/compat.c, set_app_icon).
func writeC(path string, imgs []*image.NRGBA) error {
	var b bytes.Buffer
	b.WriteString("// modern-renderer/appicon.c\n// The Clusterball Re-Enhanced window icon as PNGs, written by assets/cmd/icon -c; do not edit.\n\n")
	b.WriteString("#include <stddef.h>\n\n")
	var sizes []int
	for _, im := range imgs {
		n := im.Bounds().Dx()
		sizes = append(sizes, n)
		var p bytes.Buffer
		png.Encode(&p, im)
		fmt.Fprintf(&b, "static const unsigned char icon_%d[%d] = {", n, p.Len())
		for i, c := range p.Bytes() {
			if i%24 == 0 {
				b.WriteString("\n   ")
			}
			fmt.Fprintf(&b, " %d,", c)
		}
		b.WriteString("\n};\n")
	}
	b.WriteString("\n// Largest first.\nconst struct { int size; const unsigned char *png; size_t len; } g_app_icons[] = {\n")
	for _, n := range sizes {
		fmt.Fprintf(&b, "    {%d, icon_%d, sizeof icon_%d},\n", n, n, n)
	}
	fmt.Fprintf(&b, "};\nconst int g_app_icon_count = %d;\n", len(sizes))
	return os.WriteFile(path, b.Bytes(), 0o644)
}
