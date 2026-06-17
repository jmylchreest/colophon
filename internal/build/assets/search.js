// search.js — the browser reader for a colophon static search index. Dependency-free ES module.
//
// It mirrors the Go engine (../docs/design/search.md): it loads the manifest once, fetches only
// the shards a query's terms touch (decompressing them in-browser), scores BM25, and fetches
// fragments only for the results it shows. The analyze() function MUST stay byte-for-byte
// equivalent to Go's search.Analyze — the shared testdata/analyzer.json golden vectors guard that.

// analyze tokenizes text with the v1 "simple-1" rules: lowercase, then split on any run of
// non-(letter|number). Mirrors Go's strings.ToLower + FieldsFunc(unicode.IsLetter|IsNumber).
export function analyze(text) {
  return text.toLowerCase().split(/[^\p{L}\p{N}]+/u).filter(Boolean);
}

// createReader returns a search reader over an index whose manifest.json lives at opts.base.
// opts.fetch overrides the global fetch (used in tests). The reader caches the manifest and every
// shard/fragment it loads.
export function createReader(opts) {
  const base = opts.base.endsWith('/') ? opts.base : opts.base + '/';
  const doFetch = opts.fetch || ((url) => fetch(url));
  const shardCache = new Map();
  const fragCache = new Map();
  let manifest = null;

  async function loadManifest() {
    if (!manifest) {
      const res = await doFetch(base + 'manifest.json');
      manifest = await res.json();
    }
    return manifest;
  }

  function shardForTerm(m, term) {
    const fc = [...term][0];
    for (const s of m.shards) {
      if (fc >= s.lo && fc <= s.hi) return s;
    }
    return null;
  }

  async function loadShard(file) {
    if (shardCache.has(file)) return shardCache.get(file);
    const res = await doFetch(base + file);
    // Shards are gzipped at rest; decompress in-browser so no server Content-Encoding is needed.
    const stream = res.body.pipeThrough(new DecompressionStream('gzip'));
    const text = await new Response(stream).text();
    const data = JSON.parse(text);
    shardCache.set(file, data);
    return data;
  }

  async function loadFragment(file) {
    if (fragCache.has(file)) return fragCache.get(file);
    const res = await doFetch(base + file);
    const data = await res.json();
    fragCache.set(file, data);
    return data;
  }

  // query ranks documents against q with BM25 and returns up to limit results (all if limit <= 0),
  // each with { id, url, title, excerpt, meta, score }. Identical math + tie-break to Go's Search.
  async function query(q, limit = 10) {
    const m = await loadManifest();
    const terms = [...new Set(analyze(q))];
    const N = m.docCount;
    const { k1, b } = m.bm25;
    const avgdl = m.avgdl;

    const scores = new Map();
    for (const term of terms) {
      const s = shardForTerm(m, term);
      if (!s) continue;
      const shard = await loadShard(s.file);
      const postings = shard[term];
      if (!postings) continue;
      const df = postings.length;
      const idf = Math.log(1 + (N - df + 0.5) / (df + 0.5));
      for (const [docId, tf] of postings) {
        const dl = m.docs[docId].len;
        const denom = tf + k1 * (1 - b + (b * dl) / avgdl);
        scores.set(docId, (scores.get(docId) || 0) + (idf * (tf * (k1 + 1))) / denom);
      }
    }

    const ranked = [...scores.entries()].sort(
      (a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : 1)
    );
    const top = limit > 0 ? ranked.slice(0, limit) : ranked;

    const results = [];
    for (const [docId, score] of top) {
      const frag = await loadFragment(m.docs[docId].frag);
      results.push({
        id: docId,
        url: frag.url,
        title: frag.title,
        excerpt: frag.excerpt,
        meta: frag.meta || {},
        score,
      });
    }
    return results;
  }

  return { query, analyze, loadManifest };
}
