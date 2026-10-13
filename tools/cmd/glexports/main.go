// glexports opengl32.dll Clusterball.exe: checks that a proxy opengl32.dll exports every OpenGL function the exe imports.
package main

import (
	"bytes"
	"debug/pe"
	"fmt"
	"os"
	"strings"
)

func main() {
	raw, _ := os.ReadFile(os.Args[1])
	exe, _ := pe.Open(os.Args[2])
	syms, _ := exe.ImportedSymbols()
	n, miss := 0, 0
	for _, s := range syms {
		p := strings.SplitN(s, ":", 2)
		if strings.EqualFold(p[1], "opengl32.dll") {
			n++
			if !bytes.Contains(raw, []byte("\x00"+p[0]+"\x00")) {
				fmt.Println("MISSING", p[0])
				miss++
			}
		}
	}
	fmt.Println("needed:", n, "missing:", miss)
}
