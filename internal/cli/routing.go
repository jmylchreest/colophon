package cli

import "io/fs"

// deliverMode is how the output tree is partitioned for one publisher in a publish run.
type deliverMode int

const (
	deliverFull   deliverMode = iota // the whole tree (routing not active)
	deliverRouted                    // only the files the router keeps for this publisher
	deliverSkip                      // nothing — a route target whose routing didn't activate
)

// routeDecision decides what a publisher receives. A route target whose routing is not active
// (no public URL resolved, or it isn't deploying) is skipped rather than handed a full mirror
// of the site — its assets stay co-located with the default publisher instead.
func routeDecision(routerActive, owns, isRouteTarget bool) deliverMode {
	if isRouteTarget && (!routerActive || !owns) {
		return deliverSkip
	}
	if routerActive {
		return deliverRouted
	}
	return deliverFull
}

// selectFS wraps a base filesystem and exposes only the files for which keep returns true.
// Directories are always traversable (so fs.WalkDir descends), but a non-kept file is
// hidden from both Open and directory listings — so a publisher walking the tree never
// sees files routed to a different publisher. keep receives slash-separated paths.
type selectFS struct {
	base fs.FS
	keep func(path string) bool
}

func (s selectFS) Open(name string) (fs.File, error) {
	f, err := s.base.Open(name)
	if err != nil || name == "." {
		return f, err
	}
	// Open first, then stat the open file: one fewer path resolution than a separate
	// fs.Stat for the common (kept) case. A non-kept file is hidden as if it didn't exist.
	if info, statErr := f.Stat(); statErr == nil && !info.IsDir() && !s.keep(name) {
		_ = f.Close()
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return f, nil
}

func (s selectFS) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(s.base, name)
	if err != nil {
		return nil, err
	}
	kept := entries[:0]
	for _, e := range entries {
		p := e.Name()
		if name != "." {
			p = name + "/" + p
		}
		if e.IsDir() || s.keep(p) {
			kept = append(kept, e)
		}
	}
	return kept, nil
}
