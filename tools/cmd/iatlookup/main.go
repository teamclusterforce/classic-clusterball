// iatlookup file.exe address [all]: names the import whose IAT slot is at the given address (or lists all slots).
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
	want, _ := strconv.ParseUint(os.Args[2], 0, 32)
	rd := func(rva uint32, n int) []byte {
		for _, s := range f.Sections {
			if rva >= s.VirtualAddress && rva < s.VirtualAddress+s.VirtualSize {
				d, _ := s.Data()
				o := int(rva - s.VirtualAddress)
				e := o + n
				if e > len(d) {
					e = len(d)
				}
				return d[o:e]
			}
		}
		return nil
	}
	cstr := func(b []byte) string {
		for i, c := range b {
			if c == 0 {
				return string(b[:i])
			}
		}
		return string(b)
	}
	imp := oh.DataDirectory[1]
	for p := imp.VirtualAddress; ; p += 20 {
		d := rd(p, 20)
		if len(d) < 20 || binary.LittleEndian.Uint32(d[12:]) == 0 {
			break
		}
		dll := cstr(rd(binary.LittleEndian.Uint32(d[12:]), 64))
		ot := binary.LittleEndian.Uint32(d[0:])
		ft := binary.LittleEndian.Uint32(d[16:])
		if ot == 0 {
			ot = ft
		}
		for i := uint32(0); ; i++ {
			e := binary.LittleEndian.Uint32(rd(ot+i*4, 4))
			if e == 0 {
				break
			}
			addr := oh.ImageBase + ft + i*4
			name := cstr(rd(e+2, 64))
			if uint64(addr) == want || len(os.Args) > 3 {
				fmt.Printf("%08x %s!%s\n", addr, dll, name)
			}
		}
	}
}
