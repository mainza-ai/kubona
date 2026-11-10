'use client';

import React, { useRef, useState } from 'react';
import { ArrowUp, Cpu, Paperclip, X } from 'lucide-react';

type Attachment = { name: string };

export function Composer({
  onSubmit,
  contextLabel,
  onClearContext,
  onAttach,
  onDetach,
  onOpenModel,
  modelLabel = 'Model',
}: {
  onSubmit: (text: string) => Promise<void> | void;
  contextLabel?: string | null;
  onClearContext?: () => void;
  onAttach?: (name: string, text: string) => void;
  onDetach?: (name: string) => void;
  onOpenModel?: () => void;
  modelLabel?: string;
}) {
  const [text, setText] = useState('');
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [busy, setBusy] = useState(false);
  const [attachError, setAttachError] = useState<string | null>(null);
  const areaRef = useRef<HTMLTextAreaElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

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
      setAttachments([]);
      if (areaRef.current) areaRef.current.style.height = 'auto';
    } finally {
      setBusy(false);
    }
  };

  const attach = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    setAttachError(null);
    try {
      const raw = await file.text();
      const textValue = raw.length > 4000 ? `${raw.slice(0, 4000)}\n…[truncated]` : raw;
      setAttachments((previous) => [...previous.filter((item) => item.name !== file.name), { name: file.name }]);
      onAttach?.(file.name, textValue);
    } catch {
      setAttachError(`Could not read ${file.name}: binary or unreadable file.`);
    }
  };

  const detach = (name: string) => {
    setAttachments((previous) => previous.filter((item) => item.name !== name));
    onDetach?.(name);
  };

  return (
    <form
      className="shrink-0 border-t border-white/[0.08] bg-[#0A0C10] px-4 py-3"
      onSubmit={(event) => { event.preventDefault(); void send(); }}
      aria-label="Composer"
    >
      {(contextLabel || attachments.length > 0) && (
        <div className="mb-2 flex flex-wrap items-center gap-1.5">
          {contextLabel && (
            <span className="inline-flex max-w-full items-center gap-1 rounded-full border border-white/10 bg-white/[0.05] px-2 py-0.5 text-[12px] text-slate-300">
              <span className="truncate">{contextLabel}</span>
              <button type="button" aria-label="Clear context" onClick={onClearContext} className="text-slate-500 hover:text-white">
                <X className="h-3 w-3" />
              </button>
            </span>
          )}
          {attachments.map((attachment) => (
            <span key={attachment.name} className="inline-flex max-w-full items-center gap-1 rounded-full border border-cyan-400/20 bg-cyan-400/5 px-2 py-0.5 text-[12px] text-cyan-100">
              <Paperclip className="h-3 w-3 shrink-0" />
              <span className="truncate">{attachment.name}</span>
              <button type="button" aria-label={`Remove ${attachment.name}`} onClick={() => detach(attachment.name)} className="text-cyan-300/70 hover:text-white">
                <X className="h-3 w-3" />
              </button>
            </span>
          ))}
        </div>
      )}
      <div className="flex items-end gap-2 rounded-2xl border border-white/10 bg-white/[0.03] px-3 py-2 focus-within:border-cyan-400/50">
        <input ref={fileRef} type="file" className="hidden" onChange={(event) => { void attach(event); }} aria-label="Attach a text file" />
        <button
          type="button"
          aria-label="Attach a text file"
          title="Attach a text file (4,000 characters)"
          onClick={() => fileRef.current?.click()}
          className="mb-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-xl text-slate-400 hover:bg-white/[0.06] hover:text-white"
        >
          <Paperclip className="h-4 w-4" />
        </button>
        <textarea
          ref={areaRef}
          rows={1}
          value={text}
          onChange={(event) => { setText(event.target.value); grow(); }}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && !event.shiftKey) {
              event.preventDefault();
              if (event.nativeEvent?.isComposing || event.keyCode === 229) return;
              void send();
            }
          }}
          aria-label="Ask Kubona"
          placeholder="Ask Kubona about this campaign…"
          className="max-h-[132px] min-h-6 flex-1 resize-none bg-transparent py-1 text-[13px] text-slate-100 placeholder:text-slate-500 focus:outline-none"
        />
        <button
          type="button"
          aria-label="Open Model Hub"
          title="Open Model Hub"
          onClick={onOpenModel}
          className="mb-0.5 inline-flex h-8 max-w-28 shrink-0 items-center gap-1 rounded-xl px-2 text-[12px] text-slate-400 hover:bg-white/[0.06] hover:text-white"
        >
          <Cpu className="h-3.5 w-3.5 shrink-0" />
          <span className="truncate">{modelLabel}</span>
        </button>
        <button
          type="submit"
          disabled={!text.trim() || busy}
          aria-label="Send to copilot"
          className="mb-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-cyan-500/20 text-cyan-200 disabled:opacity-30"
        >
          <ArrowUp className="h-4 w-4" />
        </button>
      </div>
      {attachError && <p role="alert" className="mt-1 text-[11px] text-amber-300">{attachError}</p>}
      <p className="mt-1 text-[11px] text-slate-400">Enter sends · Shift+Enter newline · context stays attached</p>
    </form>
  );
}
