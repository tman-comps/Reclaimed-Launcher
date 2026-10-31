package state

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestFriends(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	st, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.SetFriends([]string{" Grippy ", "", "grippy", "Chief"})
	want := []string{"Grippy", "Chief"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, err)
	}
	st, _ = Open(p)
	if !reflect.DeepEqual(st.Friends(), want) {
		t.Fatal("not saved:", st.Friends())
	}
}
