import { flushSync } from 'svelte';
import { app } from './store.svelte';
import { reducedMotion } from './motion.svelte';

export const morphState = $state({ active: false });

let morphEl: HTMLElement | null = null;
let flight: Animation | null = null;

const MORPH_MS = 480;
const MORPH_EASING = 'cubic-bezier(0.22, 1, 0.36, 1)';

export function setMorphElement(el: HTMLElement | null) {
  morphEl = el;
}

function clearInline(el: HTMLElement) {
  el.style.width = '';
  el.style.height = '';
  el.style.minWidth = '';
  el.style.maxWidth = '';
  el.style.minHeight = '';
  el.style.maxHeight = '';
  el.style.borderRadius = '';
  el.style.backgroundColor = '';
}

function freeze(el: HTMLElement, f: Frame) {
  el.style.width = f.width;
  el.style.height = f.height;
  el.style.minWidth = '0';
  el.style.maxWidth = 'none';
  el.style.minHeight = '0';
  el.style.maxHeight = 'none';
  el.style.borderRadius = f.borderRadius;
  el.style.backgroundColor = f.backgroundColor;
}

type Frame = ReturnType<typeof frame>;

function frame(el: HTMLElement) {
  const rect = el.getBoundingClientRect();
  const cs = getComputedStyle(el);
  return {
    width: `${rect.width}px`,
    height: `${rect.height}px`,
    borderRadius: cs.borderRadius,
    backgroundColor: cs.backgroundColor,
  };
}

export function setAuthenticated(target: boolean) {
  const el = morphEl;
  if (!el || app.authenticated === target) {
    app.authenticated = target;
    return;
  }
  if (reducedMotion.current) {
    flight?.cancel();
    flight = null;
    morphState.active = false;
    app.authenticated = target;
    return;
  }
  flight?.cancel();
  const from = frame(el);
  morphState.active = true;
  flushSync(() => {
    app.authenticated = target;
  });
  const to = frame(el);
  freeze(el, from);
  const player = el.animate([from, to], { duration: MORPH_MS, easing: MORPH_EASING });
  freeze(el, to);
  flight = player;
  player.finished
    .then(() => {
      if (flight !== player) return;
      flight = null;
      clearInline(el);
      morphState.active = false;
    })
    .catch(() => {});
}
