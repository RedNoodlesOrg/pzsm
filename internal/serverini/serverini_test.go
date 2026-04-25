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

func TestRead(t *testing.T) {
	input := "# Players can hurt and kill other players\n" +
		"PVP=true\n" +
		"\n" +
		"# multi-line comment\n" +
		"# second line\n" +
		"PauseEmpty=true\n" +
		"\n" +
		"PublicName=My PZ Server\n" +
		"WorkshopItems=1;2;3\n"
	path := writeFixture(t, input)

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := []Entry{
		{Key: "PVP", Value: "true", Comment: "Players can hurt and kill other players"},
		{Key: "PauseEmpty", Value: "true", Comment: "multi-line comment\nsecond line"},
		{Key: "PublicName", Value: "My PZ Server", Comment: ""},
		{Key: "WorkshopItems", Value: "1;2;3", Comment: ""},
	}
	if len(got) != len(want) {
		t.Fatalf("len: want %d, got %d (%+v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: want %+v, got %+v", i, want[i], got[i])
		}
	}
}

func TestRead_HandlesCRLF(t *testing.T) {
	input := strings.ReplaceAll("# c\nKey=Value\n", "\n", "\r\n")
	path := writeFixture(t, input)
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 || got[0].Key != "Key" || got[0].Value != "Value" || got[0].Comment != "c" {
		t.Errorf("unexpected entries: %+v", got)
	}
}

func TestRead_BlankLineSeparatesComment(t *testing.T) {
	// Comments that don't directly precede a key should not attach.
	input := "# orphan comment\n\nKey=Value\n"
	path := writeFixture(t, input)
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 || got[0].Comment != "" {
		t.Errorf("expected detached comment, got %+v", got)
	}
}

func TestRead_MissingFile(t *testing.T) {
	_, err := Read(filepath.Join(t.TempDir(), "nope.ini"))
	if err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("expected read error, got %v", err)
	}
}

func TestWriteFields(t *testing.T) {
	path := writeFixture(t, fixtureLF)
	if err := WriteFields(path, map[string]string{
		"Map":        "RiverbendCove",
		"PublicName": "New Name",
	}); err != nil {
		t.Fatalf("WriteFields: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := "Mods=ETO_FPS;NVAPI;errorMagnifier;tsarslib\n" +
		"Map=RiverbendCove\n" +
		"PublicName=New Name\n" +
		"WorkshopItems=3119788162;2776633989;2896041179\n"
	if string(got) != want {
		t.Errorf("output mismatch:\nwant %q\ngot  %q", want, string(got))
	}
}

func TestWriteFields_PreservesCRLF(t *testing.T) {
	input := strings.ReplaceAll(fixtureLF, "\n", "\r\n")
	path := writeFixture(t, input)
	if err := WriteFields(path, map[string]string{"PublicName": "x"}); err != nil {
		t.Fatalf("WriteFields: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Contains(got, []byte("PublicName=x\r\n")) {
		t.Errorf("expected CRLF on rewritten line: %q", got)
	}
	if n := bytes.Count(got, []byte("\r\n")); n != 4 {
		t.Errorf("expected 4 CRLFs, got %d", n)
	}
}

func TestWriteFields_PreservesComments(t *testing.T) {
	input := "# leading comment\nPVP=true\n# another\nMap=Old\n"
	path := writeFixture(t, input)
	if err := WriteFields(path, map[string]string{"Map": "New"}); err != nil {
		t.Fatalf("WriteFields: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := "# leading comment\nPVP=true\n# another\nMap=New\n"
	if string(got) != want {
		t.Errorf("comments not preserved:\nwant %q\ngot  %q", want, string(got))
	}
}

func TestWriteFields_MissingKey(t *testing.T) {
	path := writeFixture(t, fixtureLF)
	err := WriteFields(path, map[string]string{"NotARealKey": "x"})
	if err == nil || !strings.Contains(err.Error(), "missing NotARealKey=") {
		t.Fatalf("expected missing-key error, got %v", err)
	}
}

func TestWriteFields_NoUpdates(t *testing.T) {
	path := writeFixture(t, fixtureLF)
	if err := WriteFields(path, nil); err != nil {
		t.Fatalf("WriteFields: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != fixtureLF {
		t.Errorf("file modified despite empty updates: %q", got)
	}
}

func TestWriteFields_OnlyFirstOccurrence(t *testing.T) {
	input := "Key=first\nKey=second\n"
	path := writeFixture(t, input)
	if err := WriteFields(path, map[string]string{"Key": "new"}); err != nil {
		t.Fatalf("WriteFields: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "Key=new\nKey=second\n" {
		t.Errorf("expected only first occurrence rewritten, got %q", got)
	}
}
