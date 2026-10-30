// Package discovery reads Project Reclaimer's public server list: signed
// records of live server addresses (each lives 180 seconds). Names, maps and
// players come from asking each server (package servers).
package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultEndpoints are tried in order.
var DefaultEndpoints = []string{
	"http://148.251.153.44:49175/v3/discovery",
	"http://23.159.176.140:49175/v3/discovery",
}

// Result is what the UI shows.
type Result struct {
	OK        bool      `json:"ok"`
	Servers   int       `json:"servers"`
	Masters   int       `json:"masters"`
	Endpoint  string    `json:"endpoint"`
	FetchedAt time.Time `json:"fetchedAt"`
	Error     string    `json:"error,omitempty"`

	// Records are kept raw; Addresses reads the server addresses from them.
	Records []json.RawMessage `json:"-"`
}

// Addresses returns each record's server address ("host:port"), without
// duplicates, in list order. Records it can't read are skipped.
func (r Result) Addresses() []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range r.Records {
		var rec struct {
			Record struct {
				Address string `json:"address"`
			} `json:"record"`
		}
		if json.Unmarshal(raw, &rec) != nil || rec.Record.Address == "" || seen[rec.Record.Address] {
			continue
		}
		seen[rec.Record.Address] = true
		out = append(out, rec.Record.Address)
	}
	return out
}

type payload struct {
	Records json.RawMessage `json:"records"`
	Masters json.RawMessage `json:"masters"`
}

// Fetch queries endpoints in order and returns the first good answer.
func Fetch(ctx context.Context, client *http.Client, endpoints []string) Result {
	var errs []error
	for _, ep := range endpoints {
		r, err := fetchOne(ctx, client, ep)
		if err == nil {
			return r
		}
		errs = append(errs, fmt.Errorf("%s: %w", ep, err))
	}
	return Result{FetchedAt: time.Now(), Error: errors.Join(errs...).Error()}
}

func fetchOne(ctx context.Context, client *http.Client, ep string) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep, nil)
	if err != nil {
		return Result{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, errors.New(resp.Status)
	}
	var p payload
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&p); err != nil {
		return Result{}, fmt.Errorf("bad JSON: %w", err)
	}
	if p.Records == nil {
		return Result{}, errors.New(`response has no "records"`)
	}
	recs, err := asList(p.Records)
	if err != nil {
		return Result{}, fmt.Errorf("records: %w", err)
	}
	masters, _ := asList(p.Masters)
	return Result{OK: true, Servers: len(recs), Masters: len(masters), Endpoint: ep, FetchedAt: time.Now(), Records: recs}, nil
}

// asList accepts a JSON array, an object (counted by its values) or null.
func asList(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, errors.New("neither a list nor an object")
	}
	out := make([]json.RawMessage, 0, len(obj))
	for _, v := range obj {
		out = append(out, v)
	}
	return out, nil
}
