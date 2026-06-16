package publish

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"io/fs"
	"sort"

	"github.com/jmylchreest/colophon/internal/core"
)

// Run is the generic incremental planner shared by every publisher. It reads the target's
// current state (p.Deployed), walks the build tree hashing each file (p.Hash) into a Plan —
// uploading new or changed files and deleting orphaned managed paths (those the publisher does
// not Protect) — then hands the Plan to p.Commit. A driver implements only its destination
// specifics; the diff lives here once.
func Run(ctx context.Context, tree fs.FS, p core.Publisher) (core.Result, error) {
	have, enumerable, err := p.Deployed(ctx)
	if err != nil {
		return core.Result{}, err
	}
	plan := &core.Plan{Desired: core.State{}}
	if err := fs.WalkDir(tree, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(tree, name)
		if err != nil {
			return err
		}
		h := p.Hash(name, b)
		plan.Desired[name] = h
		if !enumerable || have[name] != h {
			plan.Upload = append(plan.Upload, core.Change{Path: name, Op: core.OpUpload, Hash: h})
		}
		return nil
	}); err != nil {
		return core.Result{}, err
	}
	if enumerable {
		var orphans []string
		for name := range have {
			if _, kept := plan.Desired[name]; !kept && !p.Protected(name) {
				orphans = append(orphans, name)
			}
		}
		sort.Strings(orphans) // deterministic delete order for stable logs/tests
		for _, name := range orphans {
			plan.Delete = append(plan.Delete, core.Change{Path: name, Op: core.OpDelete})
		}
	}
	return p.Commit(ctx, tree, plan)
}

// FileWriter is the per-file apply surface implemented by object/directory backends (local,
// r2). Put writes or overwrites a file's bytes; Delete removes one.
type FileWriter interface {
	Put(ctx context.Context, name string, b []byte) error
	Delete(ctx context.Context, name string) error
}

// CommitFiles applies a Plan to a per-file backend: it uploads every plan.Upload (reading
// bytes from tree) and, when deleteOrphaned, removes every plan.Delete. Total is the size of
// the deployed tree. It is the shared Commit body for per-file publishers.
func CommitFiles(ctx context.Context, tree fs.FS, w FileWriter, plan *core.Plan, deleteOrphaned bool) (core.Result, error) {
	res := core.Result{Total: len(plan.Desired)}
	for _, c := range plan.Upload {
		b, err := fs.ReadFile(tree, c.Path)
		if err != nil {
			return res, err
		}
		if err := w.Put(ctx, c.Path, b); err != nil {
			return res, err
		}
		res.Uploaded++
		res.Bytes += int64(len(b))
	}
	if deleteOrphaned {
		for _, c := range plan.Delete {
			if err := w.Delete(ctx, c.Path); err != nil {
				return res, err
			}
			res.Deleted++
		}
	}
	return res, nil
}

// MD5Hex is the content hash for backends that compare against an object store's ETag (the
// MD5 of a non-multipart object) or a local file: lower-case hex MD5.
//
// MD5 is intentional and not a security choice: S3/R2 expose the ETag of a non-multipart
// PUT as the MD5 of the payload, so hashing with anything else would force a re-upload of
// every object on every run. This is a content fingerprint for change detection only — it
// never authenticates anything. (Static analyzers flag MD5 here; this is a false positive.)
func MD5Hex(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}
