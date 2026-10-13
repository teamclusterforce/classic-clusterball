// mrgls file.mrg: lists the entries of a Clusterball archive (Data.mrg, Local.mrg, Levels\*.mrg) with offset and length.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

func cstr(b []byte, p int) (string, int) {
	s := p
	for b[p] != 0 {
		p++
	}
	return string(b[s:p]), p + 1
}

func main() {
	b, _ := os.ReadFile(os.Args[1])
	be := binary.BigEndian
	nf := int(be.Uint32(b[4:]))
	p := 8
	for i := 0; i < nf; i++ {
		bl := int(be.Uint32(b[p:]))
		q := p + 4
		name, q2 := cstr(b, q)
		n := int(be.Uint32(b[q2:]))
		q2 += 4
		for j := 0; j < n; j++ {
			id := be.Uint32(b[q2:])
			off := be.Uint32(b[q2+4:])
			ln := be.Uint32(b[q2+8:])
			fn, nq := cstr(b, q2+12)
			fmt.Printf("%s/%d.%s off=%d len=%d\n", name, id, fn, off, ln)
			q2 = nq
		}
		p += bl
	}
}
