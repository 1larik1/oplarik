package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestStartupApproved(t *testing.T) {
	if !approvedOn(RegVal{Absent: true}) {
		t.Fatal("missing value means enabled")
	}
	off, on := approvedValue(false), approvedValue(true)
	if approvedOn(off) || !approvedOn(on) || len(off.Data) != 12 || off.Type != REG_BINARY {
		t.Fatal("approved value encoding")
	}
	if !approvedOn(RegVal{Type: REG_BINARY, Data: []byte{6, 0, 0, 0}}) || approvedOn(RegVal{Type: REG_BINARY, Data: []byte{7}}) {
		t.Fatal("Task Manager variants 06/07")
	}
}

func TestDNSPacket(t *testing.T) {
	p, id := dnsQueryPacket("api.steampowered.com")
	if binary.BigEndian.Uint16(p[0:]) != id || binary.BigEndian.Uint16(p[4:]) != 1 {
		t.Fatal("header")
	}
	want := "\x03api\x0csteampowered\x03com\x00\x00\x01\x00\x01"
	if string(p[12:]) != want {
		t.Fatalf("question %q", p[12:])
	}
	res := []DNSResult{
		{DNSServer: DNSServer{ID: "current"}, Ms: 2},
		{DNSServer: DNSServer{ID: "a"}, Ms: 20},
		{DNSServer: DNSServer{ID: "b"}, Ms: 9, Fails: 1},
		{DNSServer: DNSServer{ID: "c"}, Ms: 12},
		{DNSServer: DNSServer{ID: "d"}, Ms: -1},
	}
	if b := bestDNS(res); b != "c" {
		t.Fatal("best", b)
	}
}

func TestWalkClean(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "a", "b"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "x.bin"), make([]byte, 100), 0o644)
	_ = os.WriteFile(filepath.Join(root, "a", "b", "y.bin"), make([]byte, 50), 0o644)
	keep := filepath.Join(outside, "keep.txt")
	_ = os.WriteFile(keep, []byte("important"), 0o644)
	_ = os.Symlink(outside, filepath.Join(root, "link"))
	if b, n := walkClean(root, false); b != 150 || n != 2 {
		t.Fatal("scan", b, n)
	}
	if b, n := walkClean(root, true); b != 150 || n != 2 {
		t.Fatal("clean", b, n)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("followed a symlink out of the cache folder")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal("root removed")
	}
	if _, err := os.Stat(filepath.Join(root, "a")); !os.IsNotExist(err) {
		t.Fatal("empty subfolders should be removed")
	}
	if b, n := walkClean(filepath.Join(root, "missing"), true); b != 0 || n != 0 {
		t.Fatal("missing root")
	}
}
