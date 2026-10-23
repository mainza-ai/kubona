'use client';

// Ambient audio engine (P4): diegetic laboratory sound. Sub-audible 60 Hz
// transformer hum at rest, a soft tick for nudges, a warm two-tone chime
// for summons — never siren klaxons. Headless-safe: every method no-ops
// when WebAudio is unavailable, the context is locked (autoplay policy),
// or the operator muted. Mute persists in localStorage.

import type { DirectorSound } from './attention';

const MUTE_KEY = 'kubona-ambient-muted';

class AmbientAudio {
  private ctx: AudioContext | null = null;
  private master: GainNode | null = null;
  private humNodes: OscillatorNode[] = [];
  private unlocked = false;
  muted: boolean;

  constructor() {
    let muted = false;
    try {
      if (typeof window !== 'undefined') {
        muted = window.localStorage?.getItem(MUTE_KEY) === '1';
      }
    } catch {
      /* storage unavailable */
    }
    this.muted = muted;
  }

  /** Call from a user gesture; also auto-attempted on first pointerdown. */
  unlock(): void {
    if (this.unlocked || typeof window === 'undefined') return;
    try {
      const AC = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
      if (!AC) return;
      this.ctx = new AC();
      this.master = this.ctx.createGain();
      this.master.gain.value = 0.9;
      this.master.connect(this.ctx.destination);
      this.unlocked = true;
      this.startHum();
    } catch {
      this.ctx = null;
    }
  }

  setMuted(muted: boolean): void {
    this.muted = muted;
    try {
      if (typeof window !== 'undefined') {
        window.localStorage?.setItem(MUTE_KEY, muted ? '1' : '0');
      }
    } catch {
      /* ignore */
    }
    if (this.master && this.ctx) {
      this.master.gain.setTargetAtTime(muted ? 0 : 0.9, this.ctx.currentTime, 0.05);
    }
  }

  private startHum(): void {
    if (!this.ctx || !this.master) return;
    try {
      // 60 Hz transformer hum + airy overtone, kept sub-audible.
      const hum = this.ctx.createOscillator();
      hum.type = 'sine';
      hum.frequency.value = 60;
      const humGain = this.ctx.createGain();
      humGain.gain.value = 0.012;
      hum.connect(humGain).connect(this.master);
      hum.start();
      const air = this.ctx.createOscillator();
      air.type = 'sine';
      air.frequency.value = 120;
      const airGain = this.ctx.createGain();
      airGain.gain.value = 0.004;
      air.connect(airGain).connect(this.master);
      air.start();
      this.humNodes = [hum, air];
    } catch {
      /* headless or blocked */
    }
  }

  play(sound: DirectorSound): void {
    if (sound === 'none' || this.muted || !this.ctx || !this.master || !this.unlocked) return;
    try {
      if (sound === 'tick') this.blip(880, 0.06, 0.05);
      else this.chime();
    } catch {
      /* never break the UI for sound */
    }
  }

  private blip(freq: number, seconds: number, gain: number): void {
    const t = this.ctx!.currentTime;
    const osc = this.ctx!.createOscillator();
    osc.type = 'sine';
    osc.frequency.value = freq;
    const g = this.ctx!.createGain();
    g.gain.setValueAtTime(gain, t);
    g.gain.exponentialRampToValueAtTime(0.0001, t + seconds);
    osc.connect(g).connect(this.master!);
    osc.start(t);
    osc.stop(t + seconds + 0.02);
  }

  private chime(): void {
    // Warm brass-ish summons: low fifth, slow decay.
    if (!this.ctx || !this.master) return;
    const t = this.ctx.currentTime;
    for (const [freq, delay] of [[523.25, 0], [784.0, 0.18]] as const) {
      const osc = this.ctx.createOscillator();
      osc.type = 'triangle';
      osc.frequency.value = freq;
      const g = this.ctx.createGain();
      g.gain.setValueAtTime(0.0001, t + delay);
      g.gain.exponentialRampToValueAtTime(0.08, t + delay + 0.03);
      g.gain.exponentialRampToValueAtTime(0.0001, t + delay + 1.2);
      osc.connect(g).connect(this.master);
      osc.start(t + delay);
      osc.stop(t + delay + 1.3);
    }
  }
}

export const ambientAudio = new AmbientAudio();
