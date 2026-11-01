'use client';

import React, { useRef, useState } from 'react';
import { ArrowUp, X } from 'lucide-react';

export function Composer({
  onSubmit,
  contextLabel,
  onClearContext,
}: {
  onSubmit: (text: string) => Promise<void> | void;
  contextLabel?: string | null;
  onClearContext?: () => void;
}) {
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const areaRef = useRef<HTMLTextAreaElement>(null);

  const grow = () => {
    const el = areaRef.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(132, el.scrollHeight)}px`;
  };

  const send = async () => {
    const value = text.trim();
    if (!value || busy) return;
    setBusy(true);
    try {
      await onSubmit(value);
      setText('');
      if (areaRef.current) areaRef.current.style.height = 'auto';
    } finally {
      setBusy(false);
    }
  };

  return (
    <form
      className="shrink-0 border-t border-white/[0.08] bg-[#0A0C10] px-4 py-3"
      onSubmit={(event) => { event.preventDefault(); void send(); }}
      aria-label="Composer"
    >
      {contextLabel && (
        <div className="mb-2 flex items-center gap-1.5">
          <span className="inline-flex max-w-full items-center gap-1 rounded-full border border-white/10 bg-white/[0.05] px-2 py-0.5 text-[11px] text-slate-300">
            <span className="truncate">{contextLabel}</span>
            <button type="button" aria-label="Clear context" onClick={onClearContext} className="text-slate-500 hover:text-white">
              <X className="h-3 w-3" />
            </button>
          </span>
        </div>
      )}
      <div className="flex items-end gap-2 rounded-2xl border border-white/10 bg-white/[0.03] px-3 py-2 focus-within:border-cyan-400/50">
        <textarea
          ref={areaRef}
          rows={1}
          value={text}
          onChange={(event) => { setText(event.target.value); grow(); }}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
              event.preventDefault();
              void send();
            }
          }}
          aria-label="Ask Kubona"
          placeholder="Ask, dispatch, or search the campaign…"
          className="max-h-[132px] min-h-6 flex-1 resize-none bg-transparent py-1 text-[13px] text-slate-100 placeholder:text-slate-500 focus:outline-none"
        />
        <button
          type="submit"
          disabled={!text.trim() || busy}
          aria-label="Send to copilot"
          className="mb-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-cyan-500/20 text-cyan-200 disabled:opacity-30"
        >
          <ArrowUp className="h-4 w-4" />
        </button>
      </div>
      <p className="mt-1 text-[11px] text-slate-500">Enter sends · Shift+Enter newline · context stays attached</p>
    </form>
  );
}
