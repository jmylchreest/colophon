package generate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIPARulesAndHash(t *testing.T) {
	ps := []Pronunciation{{Word: "zebra", Say: "zebbra"}, {Word: "router", IPA: "ˈruːtə"}, {Word: "route", IPA: "rˈuːt"}}
	rules := ipaRules(ps)
	if len(rules) != 2 || rules[0].StringToReplace != "route" || rules[1].StringToReplace != "router" {
		t.Fatalf("expected sorted IPA-only rules, got %+v", rules)
	}
	if rules[0].Type != "phoneme" || rules[0].Alphabet != "ipa" {
		t.Errorf("bad rule shape: %+v", rules[0])
	}
	if rules[0].Phoneme != "/rˈuːt/" { // slash-delimited, as the dashboard editor stores it
		t.Errorf("phoneme must be slash-delimited, got %q", rules[0].Phoneme)
	}
	if rulesHash(rules) == rulesHash(ipaRules([]Pronunciation{{Word: "router", IPA: "X"}, {Word: "route", IPA: "rˈuːt"}})) {
		t.Error("hash should change when a phoneme changes")
	}
}

func TestWrapIPA(t *testing.T) {
	for in, want := range map[string]string{
		"rˈuːt":   "/rˈuːt/",
		"/rˈuːt/": "/rˈuːt/",
		"/rˈuːt":  "/rˈuːt/", // lone leading slash
		"rˈuːt/":  "/rˈuːt/", // lone trailing slash
	} {
		if got := wrapIPA(in); got != want {
			t.Errorf("wrapIPA(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRemovedWords(t *testing.T) {
	got := removedWords([]string{"a", "b", "c"}, []pronRule{{StringToReplace: "b"}})
	if !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Errorf("got %v", got)
	}
}

// Full lifecycle against a fake ElevenLabs API: create on first run, reuse (no calls) when
// unchanged, then update-in-place (add-rules + remove-rules) when the dict changes.
func TestSyncElevenLabsDict_Lifecycle(t *testing.T) {
	var calls []string
	created := []map[string]any{} // dictionaries currently on the account (reflected by list)
	version := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/pronunciation-dictionaries/add-from-rules", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "create")
		version++
		created = append(created, map[string]any{"id": "dict1", "name": "colophon:blog.example", "latest_version_id": "v1"})
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "dict1", "version_id": "v1"})
	})
	mux.HandleFunc("/pronunciation-dictionaries/dict1/add-rules", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "add")
		version++
		_ = json.NewEncoder(w).Encode(map[string]any{"version_id": fmt.Sprintf("v%d", version)})
	})
	mux.HandleFunc("/pronunciation-dictionaries/dict1/remove-rules", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RuleStrings []string `json:"rule_strings"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !reflect.DeepEqual(body.RuleStrings, []string{"route"}) {
			t.Errorf("remove-rules got %v, want [route]", body.RuleStrings)
		}
		calls = append(calls, "remove")
		version++
		_ = json.NewEncoder(w).Encode(map[string]any{"version_id": fmt.Sprintf("v%d", version)})
	})
	mux.HandleFunc("/pronunciation-dictionaries", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "list")
		_ = json.NewEncoder(w).Encode(map[string]any{"pronunciation_dictionaries": created})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	s := SpeechSettings{Driver: driverElevenLabs, BaseURL: srv.URL, APIKey: "k", SiteID: "blog.example"}
	v1 := []Pronunciation{{Word: "router", IPA: "ˈruːtə"}, {Word: "route", IPA: "rˈuːt"}, {Word: "nginx", Say: "engine x"}}

	loc, err := syncElevenLabsDict(context.Background(), s, "en_GB", v1, dir)
	if err != nil || loc == nil || loc.DictID != "dict1" || loc.VersionID != "v1" {
		t.Fatalf("create: loc=%+v err=%v", loc, err)
	}
	if !reflect.DeepEqual(calls, []string{"list", "create"}) { // cold path lists first, finds none, creates
		t.Fatalf("first run calls = %v", calls)
	}

	// Unchanged → cached, no API calls.
	if _, err := syncElevenLabsDict(context.Background(), s, "en_GB", v1, dir); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"list", "create", "list"}) {
		t.Fatalf("unchanged run calls = %v", calls)
	}

	// Changed (drop "route", change "router") → list, then add-rules + remove-rules on same id.
	v2 := []Pronunciation{{Word: "router", IPA: "ˈruːtər"}}
	loc, err = syncElevenLabsDict(context.Background(), s, "en_GB", v2, dir)
	if err != nil || loc.DictID != "dict1" {
		t.Fatalf("update: loc=%+v err=%v", loc, err)
	}
	if !reflect.DeepEqual(calls, []string{"list", "create", "list", "list", "add", "remove"}) {
		t.Fatalf("update calls = %v", calls)
	}
}

// An archived dictionary still appears in the list API but is hidden in the dashboard, so it is
// treated as gone and a fresh active one is created rather than reused.
func TestSyncElevenLabsDict_RecreatesArchived(t *testing.T) {
	var created bool
	mux := http.NewServeMux()
	mux.HandleFunc("/pronunciation-dictionaries", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"pronunciation_dictionaries": []map[string]any{
			{"id": "old", "name": "colophon:blog.example", "latest_version_id": "v1", "archived_time_unix": 123},
		}})
	})
	mux.HandleFunc("/pronunciation-dictionaries/add-from-rules", func(w http.ResponseWriter, r *http.Request) {
		created = true
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "new", "version_id": "v1"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	pron := []Pronunciation{{Word: "zebra", IPA: "ˈzɛbrə"}}
	_ = writePronDictState(filepath.Join(dir, prondictStateFile), map[string]prondictState{
		driverElevenLabs: {DictID: "old", VersionID: "v1", Hash: rulesHash(ipaRules(pron)), Words: []string{"zebra"}},
	})
	s := SpeechSettings{Driver: driverElevenLabs, BaseURL: srv.URL, APIKey: "k", SiteID: "blog.example"}
	loc, err := syncElevenLabsDict(context.Background(), s, "en_GB", pron, dir)
	if err != nil || !created || loc.DictID != "new" {
		t.Fatalf("archived dict should be recreated, created=%v loc=%+v err=%v", created, loc, err)
	}
}

// A tracked dictionary deleted out of band (absent from the account list) is recreated, not
// reused — even when the rules are unchanged.
func TestSyncElevenLabsDict_RecreatesDeleted(t *testing.T) {
	var created bool
	mux := http.NewServeMux()
	mux.HandleFunc("/pronunciation-dictionaries", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"pronunciation_dictionaries": []any{}}) // account is empty
	})
	mux.HandleFunc("/pronunciation-dictionaries/add-from-rules", func(w http.ResponseWriter, r *http.Request) {
		created = true
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "newdict", "version_id": "v1"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	pron := []Pronunciation{{Word: "zebra", IPA: "ˈzɛbrə"}}
	// State pins a dictionary that no longer exists, with a hash matching the current rules.
	_ = writePronDictState(filepath.Join(dir, prondictStateFile), map[string]prondictState{
		driverElevenLabs: {DictID: "gone", VersionID: "v9", Hash: rulesHash(ipaRules(pron)), Words: []string{"zebra"}},
	})
	s := SpeechSettings{Driver: driverElevenLabs, BaseURL: srv.URL, APIKey: "k", SiteID: "blog.example"}
	loc, err := syncElevenLabsDict(context.Background(), s, "en_GB", pron, dir)
	if err != nil || !created || loc.DictID != "newdict" {
		t.Fatalf("expected recreate of deleted dict, created=%v loc=%+v err=%v", created, loc, err)
	}
}

// On a lost state file, an existing dictionary on the account is found by its (dict-tagged)
// name and adopted (no duplicate create). A bare pre-per-language name is NOT adopted by name —
// only via the state file with a matching hash — so one language's rules can never land in
// another language's dictionary.
func TestSyncElevenLabsDict_FindByName(t *testing.T) {
	var created bool
	mux := http.NewServeMux()
	mux.HandleFunc("/pronunciation-dictionaries", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"pronunciation_dictionaries": []map[string]any{
			{"id": "other", "name": "colophon:someone-else/en_GB", "latest_version_id": "vx"},
			{"id": "mine", "name": "colophon:blog.example/en_GB", "latest_version_id": "v9"},
		}})
	})
	mux.HandleFunc("/pronunciation-dictionaries/add-from-rules", func(w http.ResponseWriter, r *http.Request) { created = true })
	mux.HandleFunc("/pronunciation-dictionaries/mine/add-rules", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"version_id": "v10"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := SpeechSettings{Driver: driverElevenLabs, BaseURL: srv.URL, APIKey: "k", SiteID: "blog.example"}
	loc, err := syncElevenLabsDict(context.Background(), s, "en_GB", []Pronunciation{{Word: "zebra", IPA: "ˈzɛbrə"}}, t.TempDir())
	if err != nil || loc.DictID != "mine" {
		t.Fatalf("expected to adopt 'mine', got loc=%+v err=%v", loc, err)
	}
	if created {
		t.Error("should not create a duplicate when one exists by name")
	}
}
