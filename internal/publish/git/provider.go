package git

import "strings"

// providerURL derives a host's Pages base URL from the parsed owner/repo, mirroring the
// per-provider public-URL discovery in the r2 driver. A host that isn't listed returns no URL,
// so the publisher falls back to the configured public_url. Add a host here to teach colophon a
// new Pages provider — no new driver needed.
var providerURL = map[string]func(owner, repo string) string{
	"github.com":   githubPagesURL,
	"gitlab.com":   gitlabPagesURL,
	"codeberg.org": codebergPagesURL,
}

// githubPagesURL: a project repo serves at https://<owner>.github.io/<repo>/, but the special
// user/org repo <owner>.github.io serves at the root. (A custom domain via a CNAME file isn't
// auto-detected — set public_url for that.)
func githubPagesURL(owner, repo string) string {
	owner = strings.ToLower(owner)
	if strings.EqualFold(repo, owner+".github.io") {
		return "https://" + owner + ".github.io/"
	}
	return "https://" + owner + ".github.io/" + repo + "/"
}

// gitlabPagesURL mirrors GitHub's project-vs-user split for gitlab.io.
func gitlabPagesURL(owner, repo string) string {
	owner = strings.ToLower(owner)
	if strings.EqualFold(repo, owner+".gitlab.io") {
		return "https://" + owner + ".gitlab.io/"
	}
	return "https://" + owner + ".gitlab.io/" + repo + "/"
}

// codebergPagesURL: Codeberg Pages serves a user's `pages` repo at <owner>.codeberg.page.
// (repo is unused — Codeberg doesn't do per-project subpaths the way GitHub/GitLab do — but the
// signature must match providerURL's function type.)
func codebergPagesURL(owner, _ string) string {
	return "https://" + strings.ToLower(owner) + ".codeberg.page/"
}

// parseRemote extracts (host, owner, repo) from a git remote URL in the common forms —
// git@host:owner/repo(.git), https://host/owner/repo(.git), ssh://git@host/owner/repo(.git).
// It returns empty strings for a local path or an unrecognised shape.
func parseRemote(repoURL string) (host, owner, repo string) {
	s := strings.TrimSuffix(strings.TrimSpace(repoURL), ".git")
	if i := strings.Index(s, "://"); i >= 0 { // scheme://[user@]host/owner/repo
		s = s[i+3:]
		if at := strings.Index(s, "@"); at >= 0 {
			s = s[at+1:]
		}
		parts := strings.SplitN(s, "/", 3)
		if len(parts) == 3 {
			return parts[0], parts[1], parts[2]
		}
		return "", "", ""
	}
	if at := strings.Index(s, "@"); at >= 0 { // git@host:owner/repo
		s = s[at+1:]
		if c := strings.Index(s, ":"); c >= 0 {
			rest := strings.SplitN(s[c+1:], "/", 2)
			if len(rest) == 2 {
				return s[:c], rest[0], rest[1]
			}
		}
	}
	return "", "", ""
}
