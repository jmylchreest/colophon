// Node parity tests for the browser reader. Run: node --test search/search.test.mjs
// (run `go test ./search/...` first — TestGenerateJSFixture writes testdata/fixture/.)
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { analyze, createReader, highlight, snippet, countMatches, trigrams, levenshtein, maxEditDist } from './search.js';

const here = dirname(fileURLToPath(import.meta.url));

test('trigrams + levenshtein mirror Go', () => {
  assert.deepEqual(trigrams('go'), ['$go', 'go$']);
  assert.deepEqual(trigrams('aaa'), ['$aa', 'aa$', 'aaa']);
  assert.equal(levenshtein('tigirs', 'tigris'), 2);
  assert.equal(levenshtein('wikilnk', 'wikilink'), 1);
  assert.equal(maxEditDist('cat'), 1);
  assert.equal(maxEditDist('storage'), 2);
});

test('highlight marks prefix matches, losslessly', () => {
  const seg = highlight('Go and wikilinks', 'wiki');
  assert.deepEqual(seg.filter((s) => s.mark).map((s) => s.text), ['wikilinks']);
  assert.equal(seg.map((s) => s.text).join(''), 'Go and wikilinks'); // segments reconstruct the input
});

test('countMatches counts prefix-matching tokens', () => {
  assert.equal(countMatches('wiki wikilink wikilinks foo', 'wiki'), 3);
  assert.equal(countMatches('nothing here', 'wiki'), 0);
});

test('snippet windows around the first match with ellipses', () => {
  const text = 'aaa '.repeat(40) + 'needle ' + 'bbb '.repeat(40);
  const seg = snippet(text, 'needle', { radius: 20 });
  assert.ok(seg.some((s) => s.mark && s.text === 'needle'), 'the match is marked');
  const joined = seg.map((s) => s.text).join('');
  assert.ok(joined.startsWith('…') && joined.endsWith('…'), 'clipped ends carry an ellipsis');
});

test('analyze matches the Go golden vectors', () => {
  const cases = JSON.parse(readFileSync(join(here, 'testdata/analyzer.json')));
  for (const c of cases) {
    assert.deepEqual(analyze(c.in), c.out, `analyze(${JSON.stringify(c.in)})`);
  }
});

test('query ranking matches Go on the emitted fixture', async () => {
  const dir = join(here, 'testdata/fixture');
  if (!existsSync(join(dir, 'manifest.json'))) {
    throw new Error('fixture missing — run `go test ./search/...` to generate testdata/fixture/');
  }
  const expected = JSON.parse(readFileSync(join(dir, 'expected.json')));
  const fetchFromDisk = async (url) => new Response(readFileSync(url));
  const reader = createReader({ base: dir + '/', fetch: fetchFromDisk });

  for (const [q, want] of Object.entries(expected)) {
    const got = await reader.query(q, 0);
    assert.equal(got.length, want.length, `result count for ${JSON.stringify(q)}`);
    for (let i = 0; i < want.length; i++) {
      assert.equal(got[i].id, want[i].id, `rank ${i} for ${JSON.stringify(q)}`);
      assert.ok(
        Math.abs(got[i].score - want[i].score) < 1e-9,
        `score for ${want[i].id} on ${JSON.stringify(q)}: ${got[i].score} vs ${want[i].score}`
      );
    }
  }
});
