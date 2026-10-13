// gen writes pass.c: pass-through stubs for every export of the system's opengl32.dll that the renderer does
// not implement itself (GLU32.dll binds to the module by name and needs all of them).
//
//	go run ./gen C:\Windows\SysWOW64\opengl32.dll gld3d.c > pass.c
package main

import (
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func cstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func main() {
	f, err := pe.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	var dd pe.DataDirectory
	switch h := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		dd = h.DataDirectory[0]
	case *pe.OptionalHeader64:
		dd = h.DataDirectory[0]
	}
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
	ed := rd(dd.VirtualAddress, 40)
	nNames := binary.LittleEndian.Uint32(ed[24:])
	namesRVA := binary.LittleEndian.Uint32(ed[32:])
	names := rd(namesRVA, int(nNames)*4)
	src, _ := os.ReadFile(os.Args[2])
	re := regexp.MustCompile(`\b(?:wgl|gl)[A-Za-z0-9]+\b`)
	wrapped := map[string]bool{}
	for _, m := range re.FindAllString(string(src), -1) {
		wrapped[m] = true
	}
	var out strings.Builder
	var list []string
	for i := 0; i < int(nNames); i++ {
		n := cstr(rd(binary.LittleEndian.Uint32(names[i*4:]), 80))
		if wrapped[n] {
			continue
		}
		list = append(list, n)
	}
	out.WriteString("// generated: pass-through stubs for every other opengl32 export\n#include <windows.h>\n")
	fmt.Fprintf(&out, "#define NPASS %d\n", len(list))
	out.WriteString("void *g_pass[NPASS + 1];\nconst char *g_pass_names[] = {\n")
	for _, n := range list {
		fmt.Fprintf(&out, "\"%s\",\n", n)
	}
	out.WriteString("0};\n")
	for i, n := range list {
		fmt.Fprintf(&out, "__asm__(\".globl _%s\\n_%s: jmp *_g_pass+%d\\n.section .drectve\\n.ascii \\\" -export:%s\\\"\\n.text\\n\");\n", n, n, i*4, n)
	}
	os.Stdout.WriteString(out.String())
	fmt.Fprintln(os.Stderr, "pass-through stubs:", len(list), "wrapped:", len(wrapped))
}
