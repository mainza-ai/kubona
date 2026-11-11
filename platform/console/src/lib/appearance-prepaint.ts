export const APPEARANCE_STORAGE_KEY = 'kubona.appearance.v1';

// Synchronous pre-paint script to avoid light/dark flash during SSR hydration.
// Kept free of react-dom / client hooks so Server Components (app/layout.tsx)
// can import it directly.
export const APPEARANCE_PREPAINT_SCRIPT = `(() => {
  try {
    const raw = localStorage.getItem(${JSON.stringify(APPEARANCE_STORAGE_KEY)});
    if (!raw) return;
    const p = JSON.parse(raw);
    const r = document.documentElement;
    if (p && p.theme === 'light') {
      r.dataset.theme = 'light';
      r.classList.remove('dark');
    } else {
      r.dataset.theme = 'dark';
      r.classList.add('dark');
    }
    if (p && typeof p.tint === 'string' && p.tint !== 'graphite' && ['sky','sage','rose','amber','teal','lavender','orchid'].includes(p.tint)) {
      r.dataset.tint = p.tint;
    }
    if (p && p.contrast === true) r.dataset.contrast = 'true';
    if (p && p.density === 'compact') r.dataset.density = 'compact';
    if (p && p.reduceMotion === true) r.dataset.reduceMotion = 'true';
  } catch {}
})();`;
