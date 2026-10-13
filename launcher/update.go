// Update-Pruefung: beim Start fragt der Launcher GitHub nach dem neuesten
// Release von Clusterball Re-Enhanced. Ist es neuer als appVersion und haengt
// ein MSI-Installer daran, bietet er an, ihn herunterzuladen und zu
// installieren. Der Download landet unter %LOCALAPPDATA%\Clusterball
// Re-Enhanced\update, wird gegen Groesse und SHA-256 aus der Release-Angabe
// geprueft, und der Launcher startet den Installer und schliesst sich, damit
// dieser seine Dateien ersetzen kann. Abschaltbar mit update_check=0 in der
// clusterball.ini.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lxn/walk"
)

const releasesAPI = "https://api.github.com/repos/teamclusterforce/classic-clusterball/releases/latest"

type ghAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"` // "sha256:<hex>", bei aelteren Releases leer
}

type ghRelease struct {
	Tag    string    `json:"tag_name"`
	Draft  bool      `json:"draft"`
	Pre    bool      `json:"prerelease"`
	Assets []ghAsset `json:"assets"`
}

// parseVersion liest "v3.1", "3.1.2" usw. als Zahlenfolge; nil, wenn etwas
// anderes als Zahlen zwischen den Punkten steht.
func parseVersion(s string) []int {
	s = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(s), "v"), "V")
	if s == "" {
		return nil
	}
	var out []int
	for _, p := range strings.Split(s, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil
		}
		out = append(out, n)
	}
	return out
}

// newerVersion: ist a neuer als b? Fehlende Stellen zaehlen als 0 (3.1 = 3.1.0).
func newerVersion(a, b string) bool {
	va, vb := parseVersion(a), parseVersion(b)
	if va == nil || vb == nil {
		return false
	}
	for i := 0; i < len(va) || i < len(vb); i++ {
		x, y := 0, 0
		if i < len(va) {
			x = va[i]
		}
		if i < len(vb) {
			y = vb[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// msiAsset ist der Installer eines Releases (die erste .msi), sonst nil.
func msiAsset(r *ghRelease) *ghAsset {
	for i := range r.Assets {
		if strings.EqualFold(filepath.Ext(r.Assets[i].Name), ".msi") {
			return &r.Assets[i]
		}
	}
	return nil
}

// availableUpdate fragt GitHub nach dem neuesten Release und liefert es samt
// Installer, wenn es neuer ist als current; sonst nil.
func availableUpdate(ctx context.Context, current string) (*ghRelease, *ghAsset, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", releasesAPI, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ClusterballCC/"+appVersion)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound { // noch kein Release
		return nil, nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("GitHub: %s", resp.Status)
	}
	var r ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return nil, nil, err
	}
	a := msiAsset(&r)
	if r.Draft || r.Pre || a == nil || !newerVersion(r.Tag, current) {
		return nil, nil, nil
	}
	return &r, a, nil
}

// downloadAsset laedt a nach dir und prueft Groesse und SHA-256. progress
// bekommt den Anteil in Prozent, nur wenn er sich aendert.
func downloadAsset(ctx context.Context, a *ghAsset, dir string, progress func(pct int)) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, filepath.Base(a.Name))
	part := dst + ".part"
	req, err := http.NewRequestWithContext(ctx, "GET", a.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "ClusterballCC/"+appVersion)
	resp, err := http.DefaultClient.Do(req) // ohne Gesamt-Timeout: der Installer ist gross
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download: %s", resp.Status)
	}
	f, err := os.Create(part)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	var got int64
	last := -1
	buf := make([]byte, 1<<20)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				os.Remove(part)
				return "", werr
			}
			h.Write(buf[:n])
			got += int64(n)
			if a.Size > 0 {
				if pct := int(got * 100 / a.Size); pct != last {
					last = pct
					progress(pct)
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(part)
			return "", rerr
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(part)
		return "", err
	}
	if err := verifyDownload(a, got, hex.EncodeToString(h.Sum(nil))); err != nil {
		os.Remove(part)
		return "", err
	}
	os.Remove(dst)
	if err := os.Rename(part, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// verifyDownload vergleicht Groesse und SHA-256 mit der Angabe des Releases.
func verifyDownload(a *ghAsset, size int64, sum string) error {
	if a.Size > 0 && size != a.Size {
		return fmt.Errorf("download unvollstaendig: %d von %d Bytes", size, a.Size)
	}
	if want, ok := strings.CutPrefix(a.Digest, "sha256:"); ok && !strings.EqualFold(want, sum) {
		return fmt.Errorf("download beschaedigt: SHA-256 %s statt %s", sum, want)
	}
	return nil
}

func updateDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "Clusterball Re-Enhanced", "update")
}

// checkForUpdate laeuft im Hintergrund und fragt bei einem neuen Release nach.
func (mw *AppWindow) checkForUpdate() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-mw.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	defer cancel()
	r, a, err := availableUpdate(ctx, appVersion)
	if err != nil || r == nil {
		return
	}
	mw.sync(func() {
		mw.setStatus(fmt.Sprintf(tr("upd.status"), r.Tag))
		msg := fmt.Sprintf(tr("upd.ask"), r.Tag, appVersion, (a.Size+(1<<20)-1)>>20)
		if walk.MsgBox(mw.MainWindow, tr("upd.title"), msg, walk.MsgBoxYesNo|walk.MsgBoxIconInformation) != walk.DlgCmdYes {
			return
		}
		go mw.installUpdate(r, a)
	})
}

// installUpdate laedt den Installer, startet ihn und schliesst den Launcher.
func (mw *AppWindow) installUpdate(r *ghRelease, a *ghAsset) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-mw.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	defer cancel()
	path, err := downloadAsset(ctx, a, updateDir(), func(pct int) {
		mw.sync(func() { mw.setStatus(fmt.Sprintf(tr("upd.loading"), r.Tag, pct)) })
	})
	mw.sync(func() {
		if err != nil {
			mw.setStatus(fmt.Sprintf(tr("upd.failed"), err))
			return
		}
		if walk.MsgBox(mw.MainWindow, tr("upd.title"), fmt.Sprintf(tr("upd.ready"), r.Tag),
			walk.MsgBoxOKCancel|walk.MsgBoxIconInformation) != walk.DlgCmdOK {
			mw.setStatus(fmt.Sprintf(tr("upd.status"), r.Tag))
			return
		}
		if err := exec.Command("msiexec", "/i", path).Start(); err != nil {
			mw.setStatus(fmt.Sprintf(tr("upd.failed"), err))
			return
		}
		mw.Close()
	})
}
