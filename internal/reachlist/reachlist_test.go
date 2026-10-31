package reachlist

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	// Trimmed from the master's real reply (docs/server-browser.md).
	const body = `{"protocol":2,"total":3,"servers":[
	 {"id":"a","signed":{"listing":{"name":"One","kind":"multiplayer","players":12,"max_players":16}}},
	 {"id":"b","signed":{"listing":{"name":"Two","kind":"firefight","players":0,"max_players":4}}},
	 {"id":"c","signed":{"listing":{"name":"Odd","players":-3}}}]}`
	c, err := parse(strings.NewReader(body))
	if err != nil || !c.OK || c.Servers != 3 || c.Players != 12 {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := parse(strings.NewReader("<html>")); err == nil {
		t.Fatal("want an error")
	}
}
