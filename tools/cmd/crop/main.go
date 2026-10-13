// crop x y w h zoom out.png in.png...: cuts the same rectangle from several images, zooms it and stacks the results.
package main

import (
	"image"
	"image/draw"
	"image/png"
	"os"
)

// crop x0 y0 w h zoom out in1 in2 ...: stacks zoomed crops vertically
func main() {
	var x0, y0, w, h, z int
	for i, p := range []*int{&x0, &y0, &w, &h, &z} {
		v := 0
		for _, c := range os.Args[1+i] {
			v = v*10 + int(c-'0')
		}
		*p = v
	}
	ins := os.Args[7:]
	out := image.NewRGBA(image.Rect(0, 0, w*z, h*z*len(ins)+4*(len(ins)-1)))
	draw.Draw(out, out.Bounds(), image.White, image.Point{}, draw.Src)
	for k, fn := range ins {
		f, _ := os.Open(fn)
		img, _ := png.Decode(f)
		f.Close()
		for y := 0; y < h*z; y++ {
			for x := 0; x < w*z; x++ {
				out.Set(x, k*(h*z+4)+y, img.At(x0+x/z, y0+y/z))
			}
		}
	}
	o, _ := os.Create(os.Args[6])
	png.Encode(o, out)
	o.Close()
}
