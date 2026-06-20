// Custom audio player for posts with `audio`/`audio_file`. Progressive enhancement: the
// markup ships a native <audio controls>; this upgrades it to a themed play/pause + scrubbable
// waveform. The waveform uses precomputed peaks (<audio>.json) when available — accurate and
// visible when paused — otherwise a live Web Audio visualiser (same-origin), else a static
// resting shape. Bars map linearly to time, so clicking/dragging seeks accurately.
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

    var peaks = null, mode = 'idle', analyser = null, freq = null, raf = 0;

    fetch(src + '.json', { cache: 'force-cache' })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (d) {
        if (d && d.peaks && d.peaks.length) { peaks = d.peaks; mode = 'static'; }
        else { mode = sameOrigin(src) ? 'live' : 'idle'; }
        draw();
      })
      .catch(function () { mode = sameOrigin(src) ? 'live' : 'idle'; draw(); });

    btn.addEventListener('click', function () { audio.paused ? audio.play() : audio.pause(); });
    audio.addEventListener('play', function () {
      btn.textContent = PAUSE; btn.setAttribute('aria-label', labelPause);
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
