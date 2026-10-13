// Readme, Lizenz und Notice von Clusterball Re-Enhanced im Spielordner. Die
// Apache-Lizenz verlangt, dass LICENSE und NOTICE jeder Weitergabe beiliegen;
// der Launcher legt sie deshalb selbst ab, unter Namen, die sich nicht mit
// eula.txt/lizenz.txt des Originalspiels verwechseln lassen. License.txt und
// Notice.txt sind Kopien von LICENSE und NOTICE im Repo-Stamm (docs_test.go
// prueft das).

package main

import (
	"embed"
	"path/filepath"
	"strings"
)

//go:embed docs/*.txt
var reDocs embed.FS

const docsPrefix = "Clusterball Re-Enhanced - "

// installDocs legt Readme, Lizenz und Notice in dir ab (Windows-Zeilenenden).
func installDocs(dir string) error {
	if dir == "" {
		return nil
	}
	files, err := reDocs.ReadDir("docs")
	if err != nil {
		return err
	}
	for _, f := range files {
		data, err := reDocs.ReadFile("docs/" + f.Name())
		if err != nil {
			return err
		}
		text := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n", "\r\n")
		if err := writeIfDifferent(filepath.Join(dir, docsPrefix+f.Name()), []byte(text)); err != nil {
			return err
		}
	}
	return nil
}
