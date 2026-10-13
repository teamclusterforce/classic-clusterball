// Ressourcen des Launchers: Manifest und Programm-Icon stecken in
// ClusterballCC.syso, das go build automatisch mitlinkt. Neu erzeugen nach
// einer Änderung an ClusterballCC.manifest oder ClusterballCC.ico (das Icon
// selbst kommt aus assets/cmd/icon):
//
//	rsrc -manifest ClusterballCC.manifest -ico ClusterballCC.ico -arch amd64 -o ClusterballCC.syso
//
// rsrc v0.10 vergibt dem Manifest die ID 1 und der Icon-Gruppe die ID 2;
// main.go setzt das Fenster-Icon über diese ID.

package main
