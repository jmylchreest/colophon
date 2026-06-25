package generate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ElevenLabs renders IPA overrides through an uploaded, versioned pronunciation dictionary
// (Say respellings are handled by text substitution in the driver and need no dictionary). We
// keep one dictionary id per account and bump its version when the rules change, tracking the
// id/version + a content hash in a small state file beside the audio cache so unchanged builds
// make no API calls. See https://elevenlabs.io/docs/api-reference/pronunciation-dictionaries.

const prondictStateFile = ".tts-prondict.json"

type elevenLabsLocator struct {
	DictID    string `json:"pronunciation_dictionary_id"`
	VersionID string `json:"version_id"`
}

// pronRule is one ElevenLabs phoneme rule (IPA alphabet).
type pronRule struct {
	Type            string `json:"type"`
	StringToReplace string `json:"string_to_replace"`
	Phoneme         string `json:"phoneme"`
	Alphabet        string `json:"alphabet"`
}

type prondictState struct {
	DictID    string   `json:"dict_id"`
	VersionID string   `json:"version_id"`
	Hash      string   `json:"hash"`
	Words     []string `json:"words"`
}

// ipaRules builds the ElevenLabs phoneme rules from a dictionary's IPA entries, sorted for a
// stable hash. Say-only entries are excluded (the driver substitutes those as text). The IPA is
// slash-delimited (/təˈmeɪtoʊ/): the dashboard editor requires that form to edit a rule, and
// ElevenLabs normalises it. (The API docs show a bare example, but bare entries can't be edited
// in the dashboard — so we send the dashboard-canonical delimited form.)
func ipaRules(ps []Pronunciation) []pronRule {
	var rules []pronRule
	for _, p := range ps {
		if p.IPA != "" {
			rules = append(rules, pronRule{Type: "phoneme", StringToReplace: p.Word, Phoneme: wrapIPA(p.IPA), Alphabet: "ipa"})
		}
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].StringToReplace < rules[j].StringToReplace })
	return rules
}

// wrapIPA delimits an IPA string with exactly one pair of slashes, tolerating input that
// already has a leading and/or trailing slash (or none).
func wrapIPA(ipa string) string {
	return "/" + strings.Trim(ipa, "/") + "/"
}

func rulesHash(rules []pronRule) string {
	h := sha256.New()
	for _, r := range rules {
		fmt.Fprintf(h, "%s\x00%s\x00%s\n", r.StringToReplace, r.Phoneme, r.Alphabet)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func ruleWords(rules []pronRule) []string {
	out := make([]string, len(rules))
	for i, r := range rules {
		out[i] = r.StringToReplace
	}
	return out
}

// removedWords returns the words present in old but absent from the current rules.
func removedWords(old []string, rules []pronRule) []string {
	keep := make(map[string]bool, len(rules))
	for _, r := range rules {
		keep[r.StringToReplace] = true
	}
	var gone []string
	for _, w := range old {
		if !keep[w] {
			gone = append(gone, w)
		}
	}
	return gone
}

// PrepareSpeech performs any one-time, provider-specific setup needed before generation and
// returns settings ready for NewSpeech. For ElevenLabs it syncs the IPA pronunciation dictionary
// (create/update/reuse, tracked in stateDir) and pins the resulting locator. For other providers
// it is a no-op. A sync failure is returned so the caller can warn and proceed without it.
func PrepareSpeech(ctx context.Context, s SpeechSettings, pron []Pronunciation, stateDir string) (SpeechSettings, error) {
	if s.Driver != driverElevenLabs {
		return s, nil
	}
	loc, err := syncElevenLabsDict(ctx, s, pron, stateDir)
	if err != nil {
		return s, err
	}
	s.elevenLabsLocator = loc
	return s, nil
}

func syncElevenLabsDict(ctx context.Context, s SpeechSettings, pron []Pronunciation, stateDir string) (*elevenLabsLocator, error) {
	rules := ipaRules(pron)
	if len(rules) == 0 {
		return nil, nil // nothing IPA to upload; Say substitution covers the rest
	}
	hash := rulesHash(rules)
	statePath := filepath.Join(stateDir, prondictStateFile)
	all := readPronDictState(statePath)
	cur := all[driverElevenLabs]

	base := s.BaseURL + "/pronunciation-dictionaries"
	hdr := map[string]string{"xi-api-key": s.APIKey}
	name := dictName(s.SiteID)

	// List the account's dictionaries once to reconcile our state with reality: a tracked
	// dictionary may have been deleted or archived out of band (recreate it), and on a cold start
	// (lost state file) one may already exist under our name (adopt it instead of orphaning a
	// duplicate). Site-id namespaces the name so multiple sites on one account don't clobber each
	// other. The sync only runs when audio is generated, so the extra call is negligible.
	dicts, err := listDicts(ctx, base, hdr)
	if err != nil {
		return nil, fmt.Errorf("list dictionaries: %w", err)
	}
	if cur.DictID != "" && !containsDictID(dicts, cur.DictID) {
		cur = prondictState{} // tracked dictionary is gone → recreate
	}
	if cur.DictID == "" {
		if found := findDict(dicts, name); found != nil {
			cur.DictID, cur.VersionID, cur.Words = found.ID, found.LatestVersionID, nil
		}
	}

	// Unchanged and still present → reuse with no further calls.
	if cur.DictID != "" && cur.Hash == hash {
		return &elevenLabsLocator{DictID: cur.DictID, VersionID: cur.VersionID}, nil
	}

	var dictID, versionID string
	if cur.DictID == "" {
		var resp struct {
			ID        string `json:"id"`
			AltID     string `json:"pronunciation_dictionary_id"`
			VersionID string `json:"version_id"`
		}
		body := map[string]any{"name": name, "rules": rules}
		if err := postJSON(ctx, base+"/add-from-rules", hdr, body, &resp); err != nil {
			return nil, fmt.Errorf("create dictionary: %w", err)
		}
		dictID = firstNonEmpty(resp.ID, resp.AltID)
		versionID = resp.VersionID
	} else {
		dictID = cur.DictID
		var resp struct {
			VersionID string `json:"version_id"`
		}
		// add-rules replaces same string_to_replace, so this covers new + changed words.
		if err := postJSON(ctx, base+"/"+dictID+"/add-rules", hdr, map[string]any{"rules": rules}, &resp); err != nil {
			return nil, fmt.Errorf("update dictionary rules: %w", err)
		}
		versionID = resp.VersionID
		if gone := removedWords(cur.Words, rules); len(gone) > 0 {
			var rresp struct {
				VersionID string `json:"version_id"`
			}
			if err := postJSON(ctx, base+"/"+dictID+"/remove-rules", hdr, map[string]any{"rule_strings": gone}, &rresp); err != nil {
				return nil, fmt.Errorf("remove dictionary rules: %w", err)
			}
			versionID = rresp.VersionID
		}
	}

	if dictID == "" {
		return nil, fmt.Errorf("dictionary id missing in response")
	}
	all[driverElevenLabs] = prondictState{DictID: dictID, VersionID: versionID, Hash: hash, Words: ruleWords(rules)}
	if err := writePronDictState(statePath, all); err != nil {
		return nil, fmt.Errorf("persist dictionary state: %w", err)
	}
	return &elevenLabsLocator{DictID: dictID, VersionID: versionID}, nil
}

// dictName is the deterministic, account-unique name for a site's dictionary, so it can be
// found again if the local state file is lost. Site-id namespaces shared accounts.
func dictName(siteID string) string {
	if siteID == "" {
		return "colophon"
	}
	return "colophon:" + siteID
}

type foundDict struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	LatestVersionID string `json:"latest_version_id"`
}

// listDicts returns the account's pronunciation dictionaries.
func listDicts(ctx context.Context, base string, hdr map[string]string) ([]foundDict, error) {
	var resp struct {
		Dicts []foundDict `json:"pronunciation_dictionaries"`
	}
	if err := getJSON(ctx, base+"?page_size=100", hdr, &resp); err != nil {
		return nil, err
	}
	return resp.Dicts, nil
}

// findDict returns the dictionary with the given name, or nil.
func findDict(dicts []foundDict, name string) *foundDict {
	for i := range dicts {
		if dicts[i].Name == name {
			return &dicts[i]
		}
	}
	return nil
}

// containsDictID reports whether a dictionary with the given id exists.
func containsDictID(dicts []foundDict, id string) bool {
	for i := range dicts {
		if dicts[i].ID == id {
			return true
		}
	}
	return false
}

func readPronDictState(path string) map[string]prondictState {
	out := map[string]prondictState{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func writePronDictState(path string, all map[string]prondictState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
