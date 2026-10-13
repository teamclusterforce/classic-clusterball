// callers file.exe va...: lists the E8/E9 rel32 calls and jumps to those addresses; callers file.exe ind slot: the
// instructions that use an IAT slot (call/jmp/mov through it).
package main

import (
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
)

func main() {
	f, _ := pe.Open(os.Args[1])
	oh := f.OptionalHeader.(*pe.OptionalHeader32)
	if os.Args[2] == "ind" {
		t, _ := strconv.ParseUint(os.Args[3], 0, 32)
		indirect(os.Args[1], uint32(t))
		return
	}
	for _, a := range os.Args[2:] {
		t, _ := strconv.ParseUint(a, 0, 32)
		for _, s := range f.Sections {
			d, _ := s.Data()
			base := uint32(oh.ImageBase) + s.VirtualAddress
			for i := 0; i+5 <= len(d); i++ {
				if d[i] != 0xE8 && d[i] != 0xE9 {
					continue
				}
				dst := base + uint32(i) + 5 + binary.LittleEndian.Uint32(d[i+1:])
				if uint64(dst) == t {
					fmt.Printf("%s: %#x %x\n", a, base+uint32(i), d[i])
				}
			}
		}
	}
}
