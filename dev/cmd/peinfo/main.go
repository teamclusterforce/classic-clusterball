// peinfo file.exe: prints a PE file's imported libraries, imported symbols and sections.
package main

import (
	"debug/pe"
	"fmt"
	"os"
	"sort"
)

func main() {
	f, err := pe.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	libs, _ := f.ImportedLibraries()
	fmt.Println("LIBS:", libs)
	syms, _ := f.ImportedSymbols()
	sort.Strings(syms)
	for _, s := range syms {
		fmt.Println(s)
	}
	for _, s := range f.Sections {
		fmt.Printf("SEC %-8s va=%08x vsz=%08x raw=%08x\n", s.Name, s.VirtualAddress, s.VirtualSize, s.Size)
	}
}
