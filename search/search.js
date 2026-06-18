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

// --- presentation helpers: query-aware highlighting + snippets over a result's text ---
// They return arrays of { text, mark } segments (not HTML), so a caller builds DOM with
// textContent and never injects markup. A token matches by the same prefix rule as search.

function queryTerms(q) {
  return [...new Set(analyze(q))];
}

// tokenSpans returns the analyzer's tokens with their offsets: [{ term, start, end }] (term
// lowercased for matching; start/end index the original text for slicing).
function tokenSpans(text) {
  const spans = [];
  for (const m of text.matchAll(/[\p{L}\p{N}]+/gu)) {
    spans.push({ term: m[0].toLowerCase(), start: m.index, end: m.index + m[0].length });
  }
  return spans;
}

function isMatch(token, terms) {
  for (const t of terms) if (token.startsWith(t)) return true; // prefix rule, as in search
  return false;
}

function segmentize(text, terms, from, to) {
  const parts = [];
  let cursor = from;
  for (const s of tokenSpans(text)) {
    if (s.end <= from || s.start >= to || !isMatch(s.term, terms)) continue;
    const st = Math.max(s.start, from);
    const en = Math.min(s.end, to);
    if (st > cursor) parts.push({ text: text.slice(cursor, st), mark: false });
    parts.push({ text: text.slice(st, en), mark: true });
    cursor = en;
  }
  if (cursor < to) parts.push({ text: text.slice(cursor, to), mark: false });
  return parts;
}

// highlight segments the whole text, marking tokens that match the query (use for titles).
export function highlight(text, query) {
  return segmentize(text, queryTerms(query), 0, text.length);
}

// snippet returns segments of a window around the first match (with "…" where clipped), or the
// text's start when nothing matches — the query-aware excerpt for a result.
export function snippet(text, query, opts = {}) {
  const radius = opts.radius ?? 90;
  const max = opts.max ?? 200;
  const terms = queryTerms(query);
  const first = tokenSpans(text).find((s) => isMatch(s.term, terms));

  let from = 0;
  let to = Math.min(text.length, max);
  if (first) {
    from = Math.max(0, first.start - radius);
    to = Math.min(text.length, first.end + radius);
    if (from > 0) {
      const sp = text.indexOf(' ', from);
      if (sp >= 0 && sp < first.start) from = sp + 1; // don't start mid-word
    }
  }
  if (to < text.length) {
    const sp = text.lastIndexOf(' ', to);
    if (sp > from) to = sp; // don't end mid-word
  }

  const parts = segmentize(text, terms, from, to);
  if (from > 0) parts.unshift({ text: '… ', mark: false });
  if (to < text.length) parts.push({ text: ' …', mark: false });
  return parts;
}

// countMatches counts the tokens in text that match the query (occurrence count for a result).
export function countMatches(text, query) {
  const terms = queryTerms(query);
  let n = 0;
  for (const s of tokenSpans(text)) if (isMatch(s.term, terms)) n++;
  return n;
}

// --- fuzzy helpers: mirror Go's trigram.go exactly (SPEC §10), shared by query + tests ---

const TRIGRAM_PAD = '$';

// trigrams returns a term's de-duplicated, sorted character trigrams over the padded term ($t$),
// by code point. Mirrors Go trigrams().
export function trigrams(term) {
  const r = [TRIGRAM_PAD, ...term, TRIGRAM_PAD];
  const seen = new Set();
  const out = [];
  for (let i = 0; i + 3 <= r.length; i++) {
    const g = r.slice(i, i + 3).join('');
    if (!seen.has(g)) {
      seen.add(g);
      out.push(g);
    }
  }
  out.sort();
  return out;
}

// levenshtein is the rune-wise edit distance. Mirrors Go levenshtein().
export function levenshtein(a, b) {
  const ra = [...a];
  const rb = [...b];
  let prev = Array.from({ length: rb.length + 1 }, (_, j) => j);
  let cur = new Array(rb.length + 1);
  for (let i = 1; i <= ra.length; i++) {
    cur[0] = i;
    for (let j = 1; j <= rb.length; j++) {
      const cost = ra[i - 1] === rb[j - 1] ? 0 : 1;
      cur[j] = Math.min(prev[j] + 1, cur[j - 1] + 1, prev[j - 1] + cost);
    }
    [prev, cur] = [cur, prev];
  }
  return prev[rb.length];
}

// maxEditDist scales the fuzzy budget to token length. Mirrors Go maxEditDist().
export function maxEditDist(term) {
  return [...term].length <= 4 ? 1 : 2;
}

// createReader returns a search reader over an index whose manifest.json lives at opts.base.
// opts.fetch overrides the global fetch (used in tests). The reader caches the manifest and every
// shard/fragment it loads.
export function createReader(opts) {
  const base = opts.base.endsWith('/') ? opts.base : opts.base + '/';
  const manifestName = opts.manifest || 'manifest.json'; // per-environment root, shards are shared
  const doFetch = opts.fetch || ((url) => fetch(url));
  const shardCache = new Map();
  const fragCache = new Map();
  let manifest = null;

  async function loadManifest() {
    if (!manifest) {
      const res = await doFetch(base + manifestName);
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

  function trigramShardFor(m, gram) {
    const fc = [...gram][0];
    for (const s of m.trigrams || []) {
      if (fc >= s.lo && fc <= s.hi) return s;
    }
    return null;
  }

  // fuzzyCandidates returns index terms within the edit-distance budget of a query token, gathered
  // from the trigram shards — the same set the Go engine derives in-memory, so results match.
  async function fuzzyCandidates(m, qt) {
    const cand = new Set();
    for (const g of trigrams(qt)) {
      const sh = trigramShardFor(m, g);
      if (!sh) continue;
      const data = await loadShard(sh.file); // same gzip-json as postings shards
      for (const t of data[g] || []) cand.add(t);
    }
    const budget = maxEditDist(qt);
    const out = [];
    for (const t of cand) if (levenshtein(qt, t) <= budget) out.push(t);
    return out;
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

    // Prefix matching: each query term matches every index term that begins with it (so "wiki"
    // finds "wikilinks"). All prefixes of a query term share its first character, hence its shard,
    // so one shard fetch per query term covers the expansion. Score the union of matched terms once
    // each, in sorted order — identical to the Go engine.
    const matched = new Map(); // index term → postings
    for (const term of terms) {
      const s = shardForTerm(m, term);
      let hit = false;
      if (s) {
        const shard = await loadShard(s.file);
        for (const indexTerm in shard) {
          if (indexTerm.startsWith(term)) {
            matched.set(indexTerm, shard[indexTerm]);
            hit = true;
          }
        }
      }
      // Fuzzy fallback: only when the token had no exact/prefix match and the index has trigrams.
      if (!hit && m.trigrams && m.trigrams.length) {
        for (const cand of await fuzzyCandidates(m, term)) {
          const cs = shardForTerm(m, cand);
          if (!cs) continue;
          const cshard = await loadShard(cs.file);
          if (cshard[cand]) matched.set(cand, cshard[cand]);
        }
      }
    }

    const scores = new Map();
    for (const term of [...matched.keys()].sort()) {
      const postings = matched.get(term);
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
        text: frag.text || '',
        meta: frag.meta || {},
        score,
      });
    }
    return results;
  }

  return { query, analyze, loadManifest };
}
