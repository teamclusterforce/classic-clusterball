// Das Programm-Icon von Clusterball.exe: der Launcher ersetzt die Icons der
// exe durch das Clusterball-Re-Enhanced-Icon (dasselbe wie ClusterballCC.ico),
// damit Explorer, Verknüpfungen und Taskleiste es zeigen. Das geschieht auf
// einer Kopie; die exe wird nur ersetzt, wenn alle Abschnitte vor den
// Ressourcen danach Byte für Byte gleich sind (dort patcht die
// Direct3D-Schicht) und das neue Icon darin steckt.

package main

import (
	"bytes"
	"debug/pe"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

//go:embed ClusterballCC.ico
var appIconICO []byte

const (
	rtIcon      = 3
	rtGroupIcon = 14
)

// resEntry ist eine Ressource: Typ, Name (Nummer oder Text) und Sprache.
type resEntry struct {
	typ   uint32
	id    uint32 // 0, wenn der Name ein Text ist
	name  string
	lang  uint16
	data  []byte
}

// readResources liest die Ressourcen der Typen in types aus einer PE-Datei.
func readResources(path string, types ...uint32) ([]resEntry, error) {
	f, err := pe.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sec := f.Section(".rsrc")
	if sec == nil {
		return nil, nil
	}
	raw, err := sec.Data()
	if err != nil {
		return nil, err
	}
	u16 := func(o uint32) uint16 { return binary.LittleEndian.Uint16(raw[o:]) }
	u32 := func(o uint32) uint32 { return binary.LittleEndian.Uint32(raw[o:]) }
	// entries liefert die Einträge eines Verzeichnisses: Name/ID und Ziel.
	entries := func(dir uint32) (ids []uint32, names []string, targets []uint32) {
		n := uint32(u16(dir+12)) + uint32(u16(dir+14))
		for i := uint32(0); i < n; i++ {
			e := dir + 16 + 8*i
			nm, off := u32(e), u32(e+4)
			if nm&0x80000000 != 0 {
				so := nm & 0x7fffffff
				l := uint32(u16(so))
				r := make([]uint16, l)
				for k := uint32(0); k < l; k++ {
					r[k] = u16(so + 2 + 2*k)
				}
				ids, names = append(ids, 0), append(names, syscall.UTF16ToString(r))
			} else {
				ids, names = append(ids, nm), append(names, "")
			}
			targets = append(targets, off)
		}
		return
	}
	want := map[uint32]bool{}
	for _, t := range types {
		want[t] = true
	}
	var out []resEntry
	tids, _, tdirs := entries(0)
	for i, t := range tids {
		if !want[t] || tdirs[i]&0x80000000 == 0 {
			continue
		}
		nids, nnames, ndirs := entries(tdirs[i] & 0x7fffffff)
		for j := range nids {
			if ndirs[j]&0x80000000 == 0 {
				continue
			}
			lids, _, ldata := entries(ndirs[j] & 0x7fffffff)
			for k := range lids {
				d := ldata[k]
				rva, size := u32(d), u32(d+4)
				if rva < sec.VirtualAddress || rva-sec.VirtualAddress+size > uint32(len(raw)) {
					return nil, fmt.Errorf("Ressource außerhalb von .rsrc")
				}
				o := rva - sec.VirtualAddress
				out = append(out, resEntry{t, nids[j], nnames[j], uint16(lids[k]), append([]byte(nil), raw[o:o+size]...)})
			}
		}
	}
	return out, nil
}

// iconGroup zerlegt eine .ico-Datei in die Bilder und das Gruppenverzeichnis
// (GRPICONDIR), dessen Einträge auf die Ressourcen-IDs firstID, firstID+1, ... zeigen.
func iconGroup(ico []byte, firstID uint16) (group []byte, images [][]byte, err error) {
	if len(ico) < 6 || binary.LittleEndian.Uint16(ico[2:]) != 1 {
		return nil, nil, errors.New("keine .ico-Datei")
	}
	n := int(binary.LittleEndian.Uint16(ico[4:]))
	var g bytes.Buffer
	g.Write(ico[:6])
	for i := 0; i < n; i++ {
		e := ico[6+16*i:]
		size, off := binary.LittleEndian.Uint32(e[8:]), binary.LittleEndian.Uint32(e[12:])
		if int(off+size) > len(ico) {
			return nil, nil, errors.New(".ico-Datei abgeschnitten")
		}
		g.Write(e[:12])
		binary.Write(&g, binary.LittleEndian, firstID+uint16(i))
		images = append(images, ico[off:off+size])
	}
	return g.Bytes(), images, nil
}

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procBeginUpdate       = kernel32.NewProc("BeginUpdateResourceW")
	procUpdateResource    = kernel32.NewProc("UpdateResourceW")
	procEndUpdateResource = kernel32.NewProc("EndUpdateResourceW")
	shell32               = syscall.NewLazyDLL("shell32.dll")
	procSHChangeNotify    = shell32.NewProc("SHChangeNotify")
)

// gameIconInstalled sagt, ob die exe schon das Re-Enhanced-Icon trägt.
func gameIconInstalled(exe string) (bool, error) {
	res, err := readResources(exe, rtIcon)
	if err != nil {
		return false, err
	}
	_, images, err := iconGroup(appIconICO, 1)
	if err != nil {
		return false, err
	}
	have := map[string]bool{}
	for _, r := range res {
		have[string(r.data)] = true
	}
	for _, im := range images {
		if !have[string(im)] {
			return false, nil
		}
	}
	return true, nil
}

// ensureGameIcon gibt Clusterball.exe das Re-Enhanced-Icon, falls sie es noch
// nicht hat.
func ensureGameIcon(exe string) error {
	if ok, err := gameIconInstalled(exe); err != nil || ok {
		return err
	}
	old, err := readResources(exe, rtIcon, rtGroupIcon)
	if err != nil {
		return err
	}
	// Die neue Gruppe übernimmt Name und Sprache der ersten alten Gruppe, die
	// Bilder bekommen IDs hinter allen vorhandenen.
	groupID, groupName, lang := uint32(1), "", uint16(0)
	maxIcon := uint32(0)
	first := true
	for _, r := range old {
		if r.typ == rtGroupIcon && first {
			groupID, groupName, lang, first = r.id, r.name, r.lang, false
		}
		if r.typ == rtIcon && r.id > maxIcon {
			maxIcon = r.id
		}
	}
	group, images, err := iconGroup(appIconICO, uint16(maxIcon+1))
	if err != nil {
		return err
	}

	tmp := exe + ".icon.tmp"
	orig, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, orig, 0o755); err != nil {
		return err
	}
	defer os.Remove(tmp)
	tp, _ := syscall.UTF16PtrFromString(tmp)
	h, _, e := procBeginUpdate.Call(uintptr(unsafe.Pointer(tp)), 0)
	if h == 0 {
		return fmt.Errorf("BeginUpdateResource: %v", e)
	}
	update := func(typ uint32, id uint32, name string, lang uint16, data []byte) error {
		nameArg := uintptr(id)
		if id == 0 {
			p, _ := syscall.UTF16PtrFromString(name)
			nameArg = uintptr(unsafe.Pointer(p))
		}
		var dp uintptr
		if len(data) > 0 {
			dp = uintptr(unsafe.Pointer(&data[0]))
		}
		if r, _, e := procUpdateResource.Call(h, uintptr(typ), nameArg, uintptr(lang), dp, uintptr(len(data))); r == 0 {
			return fmt.Errorf("UpdateResource %d/%d%s: %v", typ, id, name, e)
		}
		return nil
	}
	fail := func(err error) error {
		procEndUpdateResource.Call(h, 1) // verwerfen
		return err
	}
	for _, r := range old { // alte Icons und Gruppen entfernen
		if err := update(r.typ, r.id, r.name, r.lang, nil); err != nil {
			return fail(err)
		}
	}
	for i, im := range images {
		if err := update(rtIcon, maxIcon+1+uint32(i), "", lang, im); err != nil {
			return fail(err)
		}
	}
	if err := update(rtGroupIcon, groupID, groupName, lang, group); err != nil {
		return fail(err)
	}
	if r, _, e := procEndUpdateResource.Call(h, 0); r == 0 {
		return fmt.Errorf("EndUpdateResource: %v", e)
	}

	if err := checkPatchedExe(orig, tmp); err != nil {
		return err
	}
	if ok, err := gameIconInstalled(tmp); err != nil || !ok {
		return fmt.Errorf("neues Icon nach dem Schreiben nicht gefunden (%v)", err)
	}
	if err := os.Rename(tmp, exe); err != nil {
		return err
	}
	// Explorer das geänderte Icon melden (SHCNE_UPDATEITEM, SHCNF_PATHW).
	ep, _ := syscall.UTF16PtrFromString(exe)
	procSHChangeNotify.Call(0x2000, 0x0005, uintptr(unsafe.Pointer(ep)), 0)
	return nil
}

// checkPatchedExe vergleicht die Abschnitte vor .rsrc der neuen exe mit dem
// Original: Lage und Inhalt müssen gleich sein.
func checkPatchedExe(orig []byte, path string) error {
	a, err := pe.NewFile(bytes.NewReader(orig))
	if err != nil {
		return err
	}
	b, err := pe.Open(path)
	if err != nil {
		return err
	}
	defer b.Close()
	for _, s := range a.Sections {
		if s.Name == ".rsrc" {
			break
		}
		t := b.Section(s.Name)
		if t == nil || t.VirtualAddress != s.VirtualAddress || t.Offset != s.Offset || t.Size != s.Size {
			return fmt.Errorf("Abschnitt %s hat sich verschoben", s.Name)
		}
		x, _ := s.Data()
		y, _ := t.Data()
		if !bytes.Equal(x, y) {
			return fmt.Errorf("Abschnitt %s hat sich verändert", s.Name)
		}
	}
	return nil
}
