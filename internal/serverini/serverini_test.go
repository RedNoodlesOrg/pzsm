package serverini

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureLF = "Mods=ETO_FPS;NVAPI;errorMagnifier;tsarslib\n" +
	"Map=AZSpawn;MobsterCompound;LastMinutePrepperReloaded\n" +
	"PublicName=My PZ Server\n" +
	"WorkshopItems=3119788162;2776633989;2896041179\n"

func writeFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "servertest.ini")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestUpdateMods(t *testing.T) {
	path := writeFixture(t, fixtureLF)
	if err := UpdateMods(path, []string{"mod1", "mod2"}, []string{"workshop1", "workshop2"}); err != nil {
		t.Fatalf("UpdateMods: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := "Mods=mod1;mod2\n" +
		"Map=AZSpawn;MobsterCompound;LastMinutePrepperReloaded\n" +
		"PublicName=My PZ Server\n" +
		"WorkshopItems=workshop1;workshop2\n"
	if string(got) != want {
		t.Errorf("output mismatch:\nwant %q\ngot  %q", want, string(got))
	}
}

func TestUpdateMods_PreservesCRLF(t *testing.T) {
	input := strings.ReplaceAll(fixtureLF, "\n", "\r\n")
	path := writeFixture(t, input)
	if err := UpdateMods(path, []string{"m"}, []string{"w"}); err != nil {
		t.Fatalf("UpdateMods: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Contains(got, []byte("Mods=m\r\n")) {
		t.Errorf("expected CRLF Mods= line, got %q", got)
	}
	if !bytes.Contains(got, []byte("WorkshopItems=w\r\n")) {
		t.Errorf("expected CRLF WorkshopItems= line, got %q", got)
	}
	if n := bytes.Count(got, []byte("\r\n")); n != 4 {
		t.Errorf("expected 4 CRLFs preserved, got %d in %q", n, got)
	}
	if bytes.Contains(got, []byte("\r\r")) {
		t.Errorf("doubled \\r in output: %q", got)
	}
}

func TestUpdateMods_EmptyLists(t *testing.T) {
	path := writeFixture(t, fixtureLF)
	if err := UpdateMods(path, nil, nil); err != nil {
		t.Fatalf("UpdateMods: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := "Mods=\n" +
		"Map=AZSpawn;MobsterCompound;LastMinutePrepperReloaded\n" +
		"PublicName=My PZ Server\n" +
		"WorkshopItems=\n"
	if string(got) != want {
		t.Errorf("output mismatch:\nwant %q\ngot  %q", want, string(got))
	}
}

func TestUpdateMods_MissingMods(t *testing.T) {
	path := writeFixture(t, "Map=foo\nWorkshopItems=1\n")
	err := UpdateMods(path, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "missing Mods=") {
		t.Fatalf("expected missing Mods= error, got %v", err)
	}
}

func TestUpdateMods_MissingWorkshop(t *testing.T) {
	path := writeFixture(t, "Mods=a\nMap=foo\n")
	err := UpdateMods(path, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "missing WorkshopItems=") {
		t.Fatalf("expected missing WorkshopItems= error, got %v", err)
	}
}

func TestUpdateMods_LeadingWhitespaceMatches(t *testing.T) {
	// Python matched via .strip().startswith("Mods=") and rewrote the whole line
	// without preserving indentation; mirror that.
	path := writeFixture(t, "  Mods=old\n\tWorkshopItems=old\n")
	if err := UpdateMods(path, []string{"x"}, []string{"y"}); err != nil {
		t.Fatalf("UpdateMods: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := "Mods=x\nWorkshopItems=y\n"
	if string(got) != want {
		t.Errorf("want %q, got %q", want, string(got))
	}
}

func TestUpdateMods_MissingFile(t *testing.T) {
	err := UpdateMods(filepath.Join(t.TempDir(), "nope.ini"), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("expected read error, got %v", err)
	}
}
