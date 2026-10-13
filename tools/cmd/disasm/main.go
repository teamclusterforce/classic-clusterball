// disasm file.exe from to: 32-bit disassembly of a VA range; disasm file.exe str va...: the C strings at those VAs.
package main

import (
	"debug/pe"
	"fmt"
	"os"
	"strconv"

	"golang.org/x/arch/x86/x86asm"
)

func main() {
	if os.Args[2] == "str" {
		for _, a := range os.Args[3:] {
			cstrAt(os.Args[1], a)
		}
		return
	}
	f, _ := pe.Open(os.Args[1])
	oh := f.OptionalHeader.(*pe.OptionalHeader32)
	from, _ := strconv.ParseUint(os.Args[2], 0, 32)
	to, _ := strconv.ParseUint(os.Args[3], 0, 32)
	for _, s := range f.Sections {
		base := uint64(oh.ImageBase) + uint64(s.VirtualAddress)
		if from < base || from >= base+uint64(s.VirtualSize) {
			continue
		}
		d, _ := s.Data()
		pc := from
		for pc < to {
			in, err := x86asm.Decode(d[pc-base:], 32)
			if err != nil {
				fmt.Printf("%08x: ??\n", pc)
				pc++
				continue
			}
			fmt.Printf("%08x: %s\n", pc, x86asm.IntelSyntax(in, pc, nil))
			pc += uint64(in.Len)
		}
	}
}
