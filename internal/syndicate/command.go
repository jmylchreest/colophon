package syndicate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jmylchreest/colophon/internal/core"
)

// commandSyndicator runs a user-supplied command once per post — a webhook-like escape hatch that
// lets a site syndicate anywhere without a built-in driver. The post is passed as environment
// variables and as JSON on stdin (never interpolated into the command string, so post content
// can't inject shell). The command's first stdout line is taken as the silo URL (empty = none).
type commandSyndicator struct {
	id      string
	command string
}

func newCommandSyndicator(conf core.SyndicatorConf) (*commandSyndicator, error) {
	cmd, _ := conf.Settings["command"].(string)
	if strings.TrimSpace(cmd) == "" {
		return nil, fmt.Errorf("syndicator %q (command): set `command:`", conf.ID)
	}
	return &commandSyndicator{id: conf.ID, command: cmd}, nil
}

func (c *commandSyndicator) ID() string     { return c.id }
func (c *commandSyndicator) Driver() string { return "command" }

func (c *commandSyndicator) Syndicate(ctx context.Context, p Post) (string, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", c.command)
	cmd.Env = append(os.Environ(),
		"COLOPHON_POST_KEY="+p.Key,
		"COLOPHON_POST_URL="+p.URL,
		"COLOPHON_POST_TITLE="+p.Title,
		"COLOPHON_POST_SUMMARY="+p.Summary,
		"COLOPHON_POST_TEXT="+p.Text,
		"COLOPHON_POST_TAGS="+strings.Join(p.Tags, ","),
		"COLOPHON_POST_PUBLISHED="+p.Published,
	)
	cmd.Stdin = bytes.NewReader(payload)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg != "" {
			return "", fmt.Errorf("command syndicator %q: %w: %s", c.id, err, msg)
		}
		return "", fmt.Errorf("command syndicator %q: %w", c.id, err)
	}
	return firstLine(out.String()), nil
}

// firstLine returns the first non-empty, trimmed line of s (the silo URL), or "".
func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			return ln
		}
	}
	return ""
}
