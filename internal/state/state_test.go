package state

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"
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

func TestFavoritesAndRecent(t *testing.T) {
	st, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	st.ToggleFavorite("1.2.3.4:5", "A")
	st.ToggleFavorite("6.7.8.9:5", "B")
	st.ToggleFavorite("1.2.3.4:5", "A") // unstar
	fav, _ := st.ServerPrefs()
	if len(fav) != 1 || fav[0].Name != "B" {
		t.Fatal(fav)
	}
	now := time.Now()
	for i := 0; i < MaxRecent+2; i++ {
		st.AddRecent(fmt.Sprintf("10.0.0.%d:1", i), "s", now)
	}
	st.AddRecent("10.0.0.5:1", "again", now)
	_, rec := st.ServerPrefs()
	if len(rec) != MaxRecent || rec[0].Address != "10.0.0.5:1" || rec[0].Name != "again" || rec[1].Address != "10.0.0.9:1" {
		t.Fatal(rec)
	}
}
