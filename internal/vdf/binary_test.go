package vdf

import (
	"bytes"
	"testing"
)

func TestBinaryRoundTrip(t *testing.T) {
	// One shortcut as Steam writes it, with a 64-bit and a float value the
	// launcher doesn't know, which must survive unchanged.
	var b bytes.Buffer
	w := func(s ...any) {
		for _, x := range s {
			switch v := x.(type) {
			case byte:
				b.WriteByte(v)
			case string:
				b.WriteString(v)
				b.WriteByte(0)
			case []byte:
				b.Write(v)
			}
		}
	}
	w(byte(0), "shortcuts", byte(0), "0")
	w(byte(2), "appid", []byte{1, 2, 3, 0x84})
	w(byte(1), "AppName", "Some Game")
	w(byte(1), "Exe", `"C:\Games\x.exe"`)
	w(byte(7), "big", []byte{1, 2, 3, 4, 5, 6, 7, 8})
	w(byte(3), "f", []byte{0, 0, 0x80, 0x3f})
	w(byte(0), "tags", byte(1), "0", "favorite", byte(8))
	w(byte(8), byte(8), byte(8))
	data := b.Bytes()

	doc, err := ParseBinary(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Bytes(); !bytes.Equal(got, data) {
		t.Fatalf("round trip changed the file:\n got %q\nwant %q", got, data)
	}
	e := doc.Child("shortcuts").Child("0")
	if e.Child("appname").Str != "Some Game" || e.Child("appid").Int != 0x84030201 {
		t.Fatalf("values: %+v", e.Children)
	}
}

func TestBinaryRejectsTruncated(t *testing.T) {
	for _, data := range [][]byte{
		{0, 's', 0},             // map never ends
		{1, 'k', 0, 'v'},        // string never ends
		{2, 'k', 0, 1, 2},       // int cut short
		{0x05, 'k', 0, 0, 0, 8}, // wide strings aren't supported
		{8, 8},                  // bytes after the end
	} {
		if _, err := ParseBinary(data); err == nil {
			t.Errorf("%v: want an error", data)
		}
	}
}
