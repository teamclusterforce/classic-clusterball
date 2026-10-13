package main

import (
	"debug/pe"
	"encoding/binary"
	"fmt"
)

// indirect lists FF 15 / FF 25 / A1 / 8B xx references to an IAT slot.
func indirect(path string, slot uint32) {
	f, _ := pe.Open(path)
	oh := f.OptionalHeader.(*pe.OptionalHeader32)
	for _, s := range f.Sections {
		d, _ := s.Data()
		base := uint32(oh.ImageBase) + s.VirtualAddress
		for i := 0; i+6 <= len(d); i++ {
			if binary.LittleEndian.Uint32(d[i+2:]) == slot && (d[i] == 0xFF || d[i] == 0x8B) {
				fmt.Printf("%#x: %x %x\n", base+uint32(i), d[i], d[i+1])
			}
			if d[i] == 0xA1 && binary.LittleEndian.Uint32(d[i+1:]) == slot {
				fmt.Printf("%#x: a1\n", base+uint32(i))
			}
		}
	}
}
