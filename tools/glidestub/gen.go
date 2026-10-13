package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	f, _ := os.Open("names.txt")
	s := bufio.NewScanner(f)
	fmt.Println("// glide2x stub: reports no 3dfx hardware so Clusterball falls back to OpenGL")
	fmt.Println("#include <windows.h>")
	fmt.Println("BOOL WINAPI DllMain(HINSTANCE h, DWORD r, LPVOID p) { return TRUE; }")
	for s.Scan() {
		n := strings.TrimSpace(s.Text())
		i := strings.LastIndex(n, "@")
		args := n[i+1:]
		fmt.Printf("__asm__(\".globl _%s\\n_%s: xor %%eax, %%eax\\nret $%s\\n.section .drectve\\n.ascii \\\" -export:%s\\\"\\n.text\\n\");\n", n, n, args, n)
	}
}
