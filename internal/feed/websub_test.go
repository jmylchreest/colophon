package feed

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
)

func TestWebSubLinks(t *testing.T) {
	s := Site{Title: "T", BaseURL: "https://b.example", Self: "https://b.example/rss.xml",
		Hubs: []string{"https://hub.example/?a=1&b=2"}}

	rss, err := RSS(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := xml.Unmarshal(rss, new(struct {
		XMLName xml.Name `xml:"rss"`
	})); err != nil {
		t.Fatalf("RSS not well-formed: %v\n%s", err, rss)
	}
	for _, want := range []string{`rel="self"`, `rel="hub"`, `a=1&amp;b=2`, `http://www.w3.org/2005/Atom`} {
		if !strings.Contains(string(rss), want) {
			t.Errorf("RSS missing %q", want)
		}
	}

	atom, err := Atom(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := xml.Unmarshal(atom, new(struct {
		XMLName xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
	})); err != nil {
		t.Fatalf("Atom not well-formed: %v", err)
	}
	for _, want := range []string{`rel="self"`, `rel="hub"`, `rel="alternate"`} {
		if !strings.Contains(string(atom), want) {
			t.Errorf("Atom missing %q", want)
		}
	}

	js, err := JSON(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(js, &root); err != nil {
		t.Fatalf("JSON not valid: %v", err)
	}
	if _, ok := root["hubs"]; !ok {
		t.Error("JSON feed missing hubs")
	}
	if root["feed_url"] != "https://b.example/rss.xml" {
		t.Errorf("JSON feed_url = %v", root["feed_url"])
	}

	// No WebSub config → no self/hub, output stays clean.
	plain, _ := RSS(Site{Title: "T", BaseURL: "https://b.example"}, nil)
	if strings.Contains(string(plain), `rel="hub"`) {
		t.Error("RSS without hubs should not emit rel=hub")
	}
}
