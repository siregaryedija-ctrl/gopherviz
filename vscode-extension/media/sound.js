// Suara 8-bit dibangkitkan dengan Web Audio (tanpa file audio). Diambil dari
// prototype/bubble-sort.html, ditambah efek untuk mode struktur.
(function (G) {
  'use strict';

  G.createSound = function (onState) {
    const FX_VOL = 0.3, MUS_VOL = 0.08, STEP = 0.24;
    let ac = null, fx, mus, noiseBuf, timer = 0, nextT = 0, stepN = 0;
    let fxOn = true, musOn = true;
    const hz = m => 440 * Math.pow(2, (m - 69) / 12);
    const ready = () => !!ac && ac.state === 'running';
    const report = () => onState(ready());

    function unlock() {
      if (!ac) {
        const AC = window.AudioContext || window.webkitAudioContext;
        if (!AC) return;
        ac = new AC();
        ac.onstatechange = report;
        const master = ac.createGain(); master.gain.value = 0.6; master.connect(ac.destination);
        fx = ac.createGain(); fx.gain.value = fxOn ? FX_VOL : 0; fx.connect(master);
        mus = ac.createGain(); mus.gain.value = musOn ? MUS_VOL : 0; mus.connect(master);
        noiseBuf = ac.createBuffer(1, Math.floor(ac.sampleRate * 0.5), ac.sampleRate);
        const d = noiseBuf.getChannelData(0);
        for (let i = 0; i < d.length; i++) d[i] = Math.random() * 2 - 1;
      }
      if (ac.state === 'suspended') ac.resume().then(report, () => {});
      report();
    }

    function tone(f, o) {
      o = o || {};
      if (!ready()) return;
      const t0 = o.when != null ? o.when : ac.currentTime + (o.at || 0), dur = o.dur || 0.08;
      const osc = ac.createOscillator(), g = ac.createGain();
      osc.type = o.type || 'square';
      osc.frequency.setValueAtTime(f, t0);
      if (o.to) osc.frequency.exponentialRampToValueAtTime(o.to, t0 + dur);
      g.gain.setValueAtTime(0.0001, t0);
      g.gain.exponentialRampToValueAtTime(o.vol || 0.5, t0 + 0.006);
      g.gain.exponentialRampToValueAtTime(0.0001, t0 + dur);
      osc.connect(g); g.connect(o.dest || fx);
      osc.start(t0); osc.stop(t0 + dur + 0.03);
    }
    function noise(o) {
      o = o || {};
      if (!ready()) return;
      const t0 = o.when != null ? o.when : ac.currentTime + (o.at || 0), dur = o.dur || 0.06;
      const src = ac.createBufferSource(), flt = ac.createBiquadFilter(), g = ac.createGain();
      src.buffer = noiseBuf; flt.type = o.ft || 'lowpass'; flt.frequency.value = o.cut || 1200;
      g.gain.setValueAtTime(o.vol || 0.4, t0);
      g.gain.exponentialRampToValueAtTime(0.0001, t0 + dur);
      src.connect(flt); flt.connect(g); g.connect(o.dest || fx);
      src.start(t0); src.stop(t0 + dur + 0.02);
    }

    const S = {
      land: () => { tone(hz(67), { dur: 0.07, vol: 0.35 }); noise({ dur: 0.04, cut: 900, vol: 0.2 }); },
      tick: () => tone(1320, { dur: 0.03, vol: 0.18 }),
      tickLow: () => tone(880, { dur: 0.03, vol: 0.14 }),
      blip: () => tone(1046, { dur: 0.05, vol: 0.22 }),
      coin: () => { tone(988, { dur: 0.05, vol: 0.22 }); tone(1319, { dur: 0.12, vol: 0.22, at: 0.05 }); },
      tok: () => { noise({ dur: 0.05, cut: 500, vol: 0.5 }); tone(196, { dur: 0.05, type: 'triangle', vol: 0.4 }); },
      jump: () => tone(392, { to: 784, dur: 0.12, vol: 0.18 }),
      gate: () => { tone(523, { dur: 0.05, vol: 0.2 }); tone(784, { dur: 0.08, vol: 0.2, at: 0.05 }); },
      gateNo: () => { tone(330, { dur: 0.08, vol: 0.25 }); tone(247, { dur: 0.14, vol: 0.25, at: 0.08 }); },
      warp: () => tone(880, { to: 220, dur: 0.25, type: 'triangle', vol: 0.4 }),
      boing: () => tone(220, { to: 880, dur: 0.2, type: 'triangle', vol: 0.45 }),
      pipe: () => [523, 392, 330, 262].forEach((f, k) => tone(f, { dur: 0.07, vol: 0.2, at: k * 0.07 })),
      door: () => { noise({ dur: 0.1, cut: 600, vol: 0.4 }); tone(147, { dur: 0.12, vol: 0.3 }); },
      appear: () => tone(660, { to: 990, dur: 0.07, vol: 0.18 }),
      yes: () => { tone(784, { dur: 0.06, vol: 0.3 }); tone(1175, { dur: 0.1, vol: 0.3, at: 0.07 }); },
      no: () => tone(330, { to: 294, dur: 0.14, type: 'triangle', vol: 0.45 }),
      thud: () => noise({ dur: 0.07, cut: 700, vol: 0.45 }),
      sparkle: () => [1568, 2093, 2637, 3136].forEach((f, k) => tone(f, { dur: 0.09, type: 'triangle', vol: 0.25, at: k * 0.055 })),
      panic: () => { noise({ dur: 0.25, cut: 300, vol: 0.7 }); tone(220, { to: 55, dur: 0.4, vol: 0.35 }); },
      think: () => { tone(523, { to: 659, dur: 0.1, type: 'triangle', vol: 0.35 }); tone(659, { to: 587, dur: 0.12, type: 'triangle', vol: 0.3, at: 0.11 }); },
      jingle: () => {
        [72, 76, 79, 84, 79, 84, 88].forEach((m, k) => tone(hz(m), { dur: k === 6 ? 0.45 : 0.1, vol: 0.24, at: 0.05 + k * 0.11 }));
        [48, 55, 60].forEach((m, k) => tone(hz(m), { dur: 0.28, type: 'triangle', vol: 0.4, at: 0.05 + k * 0.26 }));
      },
    };

    const BASS = [48, 48, 55, 55, 57, 57, 53, 55];
    const MEL = [76, 0, 79, 76, 72, 0, 74, 0, 76, 0, 79, 81, 79, 0, 76, 74, 72, 0, 74, 76, 79, 0, 76, 0, 74, 0, 72, 74, 67, 0, 72, 0];
    function schedule() {
      if (!ready()) return;
      if (nextT < ac.currentTime - 0.1) nextT = ac.currentTime + 0.05;
      while (nextT < ac.currentTime + 0.25) {
        const s = stepN % 32;
        if (s % 2 === 0) tone(hz(BASS[Math.floor(s / 4)]), { when: nextT, dur: STEP * 1.6, type: 'triangle', vol: 0.5, dest: mus });
        if (MEL[s]) tone(hz(MEL[s]), { when: nextT, dur: STEP * 0.8, vol: 0.16, dest: mus });
        if (s % 2 === 1) noise({ when: nextT, dur: 0.03, ft: 'highpass', cut: 6000, vol: 0.12, dest: mus });
        nextT += STEP; stepN++;
      }
    }

    return {
      unlock,
      ready,
      play(name) { if (fxOn && ready() && S[name]) S[name](); },
      music(on) {
        if (on && !timer) { nextT = 0; timer = setInterval(schedule, 60); }
        else if (!on && timer) { clearInterval(timer); timer = 0; }
      },
      setFx(v) { fxOn = v; if (fx) fx.gain.value = v ? FX_VOL : 0; },
      setMus(v) { musOn = v; if (mus) mus.gain.value = v ? MUS_VOL : 0; },
    };
  };
})(window.GLV = window.GLV || {});
