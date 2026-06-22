package webmention

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadEndpoint(t *testing.T) {
	cases := []struct{ source, receiver, want string }{
		{"", "https://webmention.io/blog.example.com/webmention", "https://webmention.io/api/mentions.jf2"},
		{"https://my.host/jf2", "https://ignored/x", "https://my.host/jf2"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := ReadEndpoint(c.source, c.receiver); got != c.want {
			t.Errorf("ReadEndpoint(%q,%q) = %q, want %q", c.source, c.receiver, got, c.want)
		}
	}
}

func TestJF2Type(t *testing.T) {
	for in, want := range map[string]string{"like-of": "like", "repost-of": "repost", "in-reply-to": "reply", "mention-of": "mention", "bookmark-of": "mention"} {
		if got := jf2Type(in); got != want {
			t.Errorf("jf2Type(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripTags(t *testing.T) {
	if got := stripTags("<p>Hello <b>there</b></p>"); got != "Hello there" {
		t.Errorf("stripTags = %q", got)
	}
}

func TestFetchJF2(t *testing.T) {
	var gotDomain, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDomain = r.URL.Query().Get("domain")
		gotToken = r.URL.Query().Get("token")
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		if page != "0" { // one page of data, then empty
			fmt.Fprint(w, `{"children":[]}`)
			return
		}
		fmt.Fprint(w, `{"children":[
			{"wm-property":"like-of","wm-target":"https://b.example/posts/x/","author":{"name":"Ada","url":"https://ada.example","photo":"https://ada.example/a.jpg"},"url":"https://ada.example/l/1","published":"2026-06-21"},
			{"wm-property":"in-reply-to","wm-target":"https://b.example/posts/x/","author":{"name":"Bob"},"url":"https://bob.example/n/2","content":{"text":"Nice!"},"published":"2026-06-22"},
			{"wm-property":"mention-of","wm-target":"https://b.example/posts/y/","author":{"name":"Cy"},"url":"https://cy.example/m/3","content":{"html":"<p>see <b>here</b></p>"}}
		]}`)
	}))
	defer srv.Close()

	got, err := FetchJF2(context.Background(), srv.Client(), srv.URL, "b.example", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if gotDomain != "b.example" || gotToken != "secret" {
		t.Errorf("query domain=%q token=%q", gotDomain, gotToken)
	}
	x := got["posts/x"]
	if len(x.Mentions) != 2 || x.Target != "https://b.example/posts/x/" {
		t.Fatalf("posts/x = %+v", x)
	}
	if x.Mentions[0].Type != "like" || x.Mentions[1].Type != "reply" || x.Mentions[1].Content != "Nice!" {
		t.Errorf("posts/x mentions = %+v", x.Mentions)
	}
	y := got["posts/y"]
	if len(y.Mentions) != 1 || y.Mentions[0].Content != "see here" { // html stripped to text
		t.Errorf("posts/y = %+v", y.Mentions)
	}
}
