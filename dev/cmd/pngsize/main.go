// pngsize dir maxdim: size histogram of a folder of PNGs; moves files larger than maxdim to dir\..\skip.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Prints a size histogram of PNGs in a folder and moves files larger than maxDim to <dir>\..\skip.
func main() {
	dir := os.Args[1]
	files, _ := filepath.Glob(filepath.Join(dir, "*.png"))
	hist := map[string]int{}
	skip := filepath.Join(dir, "..", "skip")
	os.MkdirAll(skip, 0o755)
	for _, f := range files {
		b := make([]byte, 24)
		h, _ := os.Open(f)
		h.Read(b)
		h.Close()
		w, ht := binary.BigEndian.Uint32(b[16:]), binary.BigEndian.Uint32(b[20:])
		hist[fmt.Sprintf("%dx%d", w, ht)]++
		if w > 1024 || ht > 1024 || w < 16 || ht < 16 {
			os.Rename(f, filepath.Join(skip, filepath.Base(f)))
		}
	}
	keys := make([]string, 0, len(hist))
	for k := range hist {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return hist[keys[i]] > hist[keys[j]] })
	for _, k := range keys {
		fmt.Printf("%5d %s\n", hist[k], k)
	}
}
