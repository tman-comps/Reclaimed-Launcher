package vdf

import "testing"

const libraryFolders = `"libraryfolders"
{
	"0"
	{
		"path"		"C:\\Program Files (x86)\\Steam"
		"label"		""
		"apps"
		{
			"228980"		"1"
		}
	}
	// second library
	"1"
	{
		"path"		"D:\\SteamLibrary"
		"apps"
		{
			"976730"		"123456"
		}
	}
}`

func TestParseLibraryFolders(t *testing.T) {
	root, err := Parse(libraryFolders)
	if err != nil {
		t.Fatal(err)
	}
	lf := root.Get("LibraryFolders")
	if lf == nil || len(lf.Keys()) != 2 {
		t.Fatalf("want 2 libraries, got %+v", lf)
	}
	if got := lf.String("1", "path"); got != `D:\SteamLibrary` {
		t.Fatalf("path = %q", got)
	}
	if lf.Get("1", "apps", "976730") == nil {
		t.Fatal("app not found")
	}
}

func TestParseErrors(t *testing.T) {
	for _, src := range []string{`"a" {`, `"a" "b`, `}`, `"a"`} {
		if _, err := Parse(src); err == nil {
			t.Errorf("expected error for %q", src)
		}
	}
}
