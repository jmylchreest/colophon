// Custom audio player for posts with `audio`/`audio_file`. Progressive enhancement: the
// markup ships a native <audio controls>; this upgrades it to a themed play/pause + scrubbable
// waveform. Waveform peaks come from, in order: a precomputed <audio>.json sidecar (recorded
// clips, or older builds), a localStorage cache, else computed in-browser from the audio itself
// (Web Audio decodeAudioData) on first play and cached — so generated readings need no second
// server render. Until peaks exist, a live visualiser (same-origin) or a resting shape shows.
// Bars map linearly to time, so clicking/dragging seeks accurately.
(function () {
  var PLAY = '▶';  // ▶
  var PAUSE = '⏸'; // ⏸

  var nodes = document.querySelectorAll('[data-audioplayer]');
  for (var i = 0; i < nodes.length; i++) init(nodes[i]);

  function init(fig) {
    var audio = fig.querySelector('audio');
    if (!audio) return;
    audio.removeAttribute('controls');
    var src = fig.getAttribute('data-src') || audio.getAttribute('src') || '';
    var labelPlay = fig.getAttribute('data-label-play') || 'Play';
    var labelPause = fig.getAttribute('data-label-pause') || 'Pause';

    var btn = el('button', 'ap-toggle');
    btn.type = 'button';
    btn.setAttribute('aria-label', labelPlay);
    btn.textContent = PLAY;
    var canvas = el('canvas', 'ap-wave');
    var time = el('span', 'ap-time');
    time.textContent = '0:00';
    fig.insertBefore(btn, audio);
    fig.insertBefore(canvas, audio);
    fig.insertBefore(time, audio);
    fig.classList.add('ap-ready');

    var cs = getComputedStyle(fig);
    var colPlayed = (cs.getPropertyValue('--accent') || '#9aa').trim();
    var colBase = (cs.getPropertyValue('--border') || '#555').trim();

    var peaks = null, mode = 'idle', analyser = null, freq = null, raf = 0, computed = false;

    fetch(src + '.json', { cache: 'force-cache' })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (d) { adoptPeaks(d && d.peaks && d.peaks.length ? d.peaks : cachedPeaks(src)); })
      .catch(function () { adoptPeaks(cachedPeaks(src)); });

    function adoptPeaks(pk) {
      if (pk && pk.length) { peaks = pk; mode = 'static'; }
      else { mode = sameOrigin(src) ? 'live' : 'idle'; }
      draw();
    }

    btn.addEventListener('click', function () { audio.paused ? audio.play() : audio.pause(); });
    audio.addEventListener('play', function () {
      btn.textContent = PAUSE; btn.setAttribute('aria-label', labelPause);
      computePeaks(); // derive the real waveform from the audio (once), now that it's being fetched
      if (mode === 'live') { resumeCtx(); cancelAnimationFrame(raf); raf = requestAnimationFrame(tick); }
    });
    audio.addEventListener('pause', function () {
      btn.textContent = PLAY; btn.setAttribute('aria-label', labelPlay);
      cancelAnimationFrame(raf); draw();
    });
    audio.addEventListener('ended', function () { btn.textContent = PLAY; });
    audio.addEventListener('timeupdate', function () { updateTime(); if (mode !== 'live') draw(); });
    audio.addEventListener('loadedmetadata', function () { updateTime(); draw(); });
    window.addEventListener('resize', draw);

    var dragging = false;
    canvas.addEventListener('pointerdown', function (e) { dragging = true; canvas.setPointerCapture(e.pointerId); seek(e.clientX); });
    canvas.addEventListener('pointermove', function (e) { if (dragging) seek(e.clientX); });
    canvas.addEventListener('pointerup', function () { dragging = false; });

    function updateTime() {
      time.textContent = fmt(audio.currentTime) + (isFinite(audio.duration) ? ' / ' + fmt(audio.duration) : '');
    }
    function seek(clientX) {
      var r = canvas.getBoundingClientRect();
      var f = Math.max(0, Math.min(1, (clientX - r.left) / r.width));
      if (isFinite(audio.duration)) audio.currentTime = f * audio.duration;
      draw();
    }
    function progress() { return isFinite(audio.duration) && audio.duration ? audio.currentTime / audio.duration : 0; }

    function draw() {
      if (mode === 'static' && peaks) bars(peaks, progress());
      else if (mode === 'live') live();
      else bars(idle(src), progress());
    }
    function bars(vals, played) {
      var c = ctx(), w = c.w, h = c.h, n = vals.length, bw = w / n;
      c.x.clearRect(0, 0, w, h);
      for (var i = 0; i < n; i++) {
        var bh = Math.max(2, vals[i] * h);
        c.x.fillStyle = (i / n) <= played ? colPlayed : colBase;
        c.x.fillRect(i * bw + bw * 0.15, (h - bh) / 2, bw * 0.7, bh);
      }
    }
    function live() {
      if (!setupCtx()) { mode = 'idle'; draw(); return; }
      analyser.getByteFrequencyData(freq);
      var vals = [], n = freq.length;
      for (var i = 0; i < n; i++) vals.push(freq[i] / 255);
      bars(vals, progress());
    }
    function tick() { if (mode === 'live' && !audio.paused) { live(); raf = requestAnimationFrame(tick); } }

    function setupCtx() {
      if (analyser) return true;
      try {
        var AC = window.AudioContext || window.webkitAudioContext;
        if (!AC) return false;
        var c = new AC();
        var s = c.createMediaElementSource(audio);
        analyser = c.createAnalyser(); analyser.fftSize = 128;
        s.connect(analyser); analyser.connect(c.destination);
        freq = new Uint8Array(analyser.frequencyBinCount);
        audio._ac = c;
        return true;
      } catch (e) { return false; }
    }
    function resumeCtx() { if (audio._ac && audio._ac.state === 'suspended') audio._ac.resume(); }

    function ctx() {
      var w = canvas.clientWidth || 320, h = canvas.clientHeight || 44, dpr = window.devicePixelRatio || 1;
      canvas.width = w * dpr; canvas.height = h * dpr;
      var x = canvas.getContext('2d'); x.setTransform(dpr, 0, 0, dpr, 0, 0);
      return { x: x, w: w, h: h };
    }
    // computePeaks fetches the audio (served from cache after playback starts) and decodes it with
    // the browser's native codec to a real 120-bar waveform — no second server render. Runs at most
    // once; result is cached in localStorage so return visits are instant. Cross-origin needs CORS
    // (our object store sends it); on any failure the live/idle visual just stays.
    function computePeaks() {
      if (computed || peaks) return;
      computed = true;
      var AC = window.AudioContext || window.webkitAudioContext;
      if (!AC || !window.fetch) return;
      fetch(src, { cache: 'force-cache' })
        .then(function (r) { return r.ok ? r.arrayBuffer() : null; })
        .then(function (buf) { return buf ? decode(new AC(), buf) : null; })
        .then(function (audioBuffer) {
          if (!audioBuffer) return;
          var pk = reduce(audioBuffer);
          if (!pk) return;
          peaks = pk; mode = 'static'; cancelAnimationFrame(raf);
          cachePeaks(src, pk); draw();
        })
        .catch(function () {});
    }
    function decode(ac, buf) {
      return new Promise(function (res, rej) {
        var p = ac.decodeAudioData(buf, res, rej); // Safari uses the callback form; modern returns a Promise
        if (p && p.then) p.then(res, rej);
      });
    }
    function reduce(ab) {
      var ch = ab.getChannelData(0), n = 120, per = ch.length / n, out = [], max = 0;
      if (ch.length < n) return null;
      for (var i = 0; i < n; i++) {
        var lo = Math.floor(i * per), hi = Math.floor((i + 1) * per), m = 0;
        for (var j = lo; j < hi; j++) { var v = ch[j] < 0 ? -ch[j] : ch[j]; if (v > m) m = v; }
        out.push(m); if (m > max) max = m;
      }
      if (max === 0) return null;
      for (var k = 0; k < n; k++) out[k] = Math.round(out[k] / max * 1000) / 1000;
      return out;
    }
    function cachePeaks(u, pk) { try { localStorage.setItem('cph.peaks.' + u, JSON.stringify(pk)); } catch (e) {} }
    function cachedPeaks(u) { try { var s = localStorage.getItem('cph.peaks.' + u); return s ? JSON.parse(s) : null; } catch (e) { return null; } }

    function sameOrigin(u) { try { return new URL(u, location.href).origin === location.origin; } catch (e) { return true; } }
    function idle(seed) {
      var h = 2166136261; for (var i = 0; i < seed.length; i++) { h ^= seed.charCodeAt(i); h = (h * 16777619) >>> 0; }
      var out = []; for (var j = 0; j < 64; j++) { h = (h * 1103515245 + 12345) >>> 0; out.push(0.2 + (h % 1000) / 1000 * 0.6); }
      return out;
    }
    function fmt(t) { if (!isFinite(t) || t < 0) t = 0; var m = Math.floor(t / 60), s = Math.floor(t % 60); return m + ':' + (s < 10 ? '0' : '') + s; }
    function el(tag, cls) { var e = document.createElement(tag); e.className = cls; return e; }
  }
})();
