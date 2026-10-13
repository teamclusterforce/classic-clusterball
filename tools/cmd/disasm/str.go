package main

import (
	"debug/pe"
	"fmt"
	"strconv"
)

// cstrAt prints the zero-terminated string at a VA.
func cstrAt(path, vaS string) {
	f, _ := pe.Open(path)
	oh := f.OptionalHeader.(*pe.OptionalHeader32)
	va, _ := strconv.ParseUint(vaS, 0, 32)
	for _, s := range f.Sections {
		base := uint64(oh.ImageBase) + uint64(s.VirtualAddress)
		if va >= base && va < base+uint64(s.Size) {
			d, _ := s.Data()
			o := va - base
			e := o
			for e < uint64(len(d)) && d[e] != 0 {
				e++
			}
			fmt.Printf("%s: %q\n", vaS, string(d[o:e]))
		}
	}
}
