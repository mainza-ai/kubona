'use client';

import React, { useEffect, useState } from 'react';
import { AlertTriangle, FileText, ShieldCheck } from 'lucide-react';
import { fetchSkills, installSkill, removeSkill, toggleSkill, type SkillsCatalog } from '@/lib/api';

type LoadState = 'loading' | 'ready' | 'unavailable';

export function SkillsSettings({ load = fetchSkills }: { load?: () => Promise<SkillsCatalog | null> }) {
  const [catalog, setCatalog] = useState<SkillsCatalog | null>(null);
  const [state, setState] = useState<LoadState>('loading');
  const [busy, setBusy] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const [installName, setInstallName] = useState('');
  const [installContent, setInstallContent] = useState('');
  const [installing, setInstalling] = useState(false);

  useEffect(() => {
    let cancelled = false;
    load()
      .then((next) => {
        if (cancelled) return;
        setCatalog(next);
        setState(next ? 'ready' : 'unavailable');
      })
      .catch(() => {
        if (cancelled) return;
        setCatalog(null);
        setState('unavailable');
      });
    return () => {
      cancelled = true;
    };
  }, [load]);

  const refresh = async () => {
    const next = await load();
    setCatalog(next);
    setState(next ? 'ready' : 'unavailable');
  };

  return (
    <section className="space-y-3" aria-labelledby="skills-settings-title">
      <header className="space-y-1">
        <h2 id="skills-settings-title" className="text-page-title font-semibold text-text">
          Runtime skills
        </h2>
        <p className="text-label-secondary text-body">
          Only skill names and descriptions enter the copilot prompt. The body is read on demand through the read-only
          <code className="mx-1 rounded-inset bg-code-chip px-1 font-code text-code-chip-ink">load_skill</code>
          tool.
        </p>
      </header>

      <details className="rounded-control border border-border bg-surface px-3 py-2">
        <summary className="cursor-pointer select-none text-label font-semibold text-text">Install a user skill</summary>
        <div className="mt-2 space-y-1.5">
          <input
            aria-label="New skill name"
            value={installName}
            onChange={(event) => setInstallName(event.target.value)}
            placeholder="skill-name"
            className="w-full rounded-inset border border-border bg-field px-2 py-1 font-code text-label text-text placeholder:text-faint"
          />
          <textarea
            aria-label="SKILL.md content"
            value={installContent}
            onChange={(event) => setInstallContent(event.target.value)}
            rows={6}
            placeholder={'---\nname: skill-name\ndescription: one line\nrole: copilot\ntools: get_anchor\n---\nbody'}
            className="w-full rounded-inset border border-border bg-field px-2 py-1 font-code text-[12px] text-text placeholder:text-faint"
          />
          <button
            type="button"
            disabled={installing || !installName.trim() || !installContent.trim()}
            onClick={async () => {
              setInstalling(true);
              setStatus(null);
              const out = await installSkill(installName.trim(), installContent);
              if (out.ok) {
                setStatus(`Installed ${installName.trim()}.`);
                setInstallName('');
                setInstallContent('');
                await refresh();
              } else {
                setStatus(out.error);
              }
              setInstalling(false);
            }}
            className="rounded-control-compact border border-border px-2 py-1 text-label text-label-secondary hover:text-text disabled:opacity-40"
          >
            {installing ? 'Installing…' : 'Install skill'}
          </button>
        </div>
      </details>

      {state === 'loading' && (
        <p role="status" className="text-label-secondary text-body">
          Loading skills…
        </p>
      )}

      {state === 'unavailable' && (
        <p role="status" className="rounded-control border border-danger-edge bg-danger-wash px-3 py-2 text-danger-ink text-body">
          Skill catalogue unavailable: the server did not return a readable catalogue. No skills are advertised to the
          copilot in this state.
        </p>
      )}

      {state === 'ready' && catalog && (
        <>
          <div className="flex items-center gap-2 text-label-secondary text-label">
            <ShieldCheck className="h-4 w-4 text-success" aria-hidden="true" />
            <span>
              {catalog.skills.length} skill{catalog.skills.length === 1 ? '' : 's'} · limit {catalog.limits.max_skills} ·{' '}
              {Math.round(catalog.limits.max_skill_bytes / 1024)} KiB each
            </span>
          </div>

          {status && <p role="status" className="text-label-secondary text-body">{status}</p>}

          {catalog.skills.length === 0 ? (
            <p className="text-label-secondary text-body">No skills are installed for the copilot role.</p>
          ) : (
            <ul className="space-y-2" aria-label="Installed skills">
              {catalog.skills.map((skill) => (
                <li key={skill.name} className="rounded-control border border-border bg-surface px-3 py-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <FileText className="h-3.5 w-3.5 text-accent" aria-hidden="true" />
                    <span className="font-code text-label text-text">{skill.name}</span>
                    <span className="rounded-chip border border-border px-1.5 py-0.5 text-caption text-label-secondary">
                      {skill.source}
                    </span>
                    <button
                      type="button"
                      disabled={busy !== null}
                      aria-pressed={skill.enabled}
                      onClick={async () => {
                        setBusy(skill.name);
                        setStatus(null);
                        const out = await toggleSkill(skill.name, !skill.enabled);
                        if (out.ok) {
                          setCatalog((prev) => prev ? {
                            ...prev,
                            skills: prev.skills.map((item) => item.name === skill.name ? { ...item, enabled: out.enabled } : item),
                          } : prev);
                          setStatus(`${skill.name}: ${out.enabled ? 'enabled' : 'disabled'}`);
                        } else {
                          setStatus(`${skill.name}: ${out.error}`);
                        }
                        setBusy(null);
                      }}
                      className="rounded-chip border border-border px-1.5 py-0.5 text-caption text-label-secondary hover:text-text disabled:opacity-40"
                    >
                      {busy === skill.name ? 'Saving…' : skill.enabled ? 'Disable' : 'Enable'}
                    </button>
                    {skill.source === 'user' && (
                      <button
                        type="button"
                        disabled={busy !== null}
                        onClick={async () => {
                          setBusy(skill.name);
                          setStatus(null);
                          const out = await removeSkill(skill.name);
                          if (out.ok) {
                            setStatus(`Removed ${skill.name}.`);
                            await refresh();
                          } else {
                            setStatus(`${skill.name}: ${out.error}`);
                          }
                          setBusy(null);
                        }}
                        className="rounded-chip border border-danger-edge px-1.5 py-0.5 text-caption text-danger-ink hover:bg-danger-wash disabled:opacity-40"
                      >
                        Remove
                      </button>
                    )}
                    {skill.tools.map((tool) => (
                      <span
                        key={`${skill.name}-${tool}`}
                        className="rounded-chip bg-fill px-1.5 py-0.5 font-code text-caption text-muted"
                      >
                        {tool}
                      </span>
                    ))}
                  </div>
                  <p className="mt-1 text-body text-label-secondary">{skill.description}</p>
                </li>
              ))}
            </ul>
          )}

          {catalog.faults.length > 0 && (
            <div
              role="alert"
              className="rounded-control border border-warning-edge bg-warning-wash px-3 py-2 text-warning-ink"
            >
              <div className="flex items-center gap-2 text-label font-semibold">
                <AlertTriangle className="h-4 w-4" aria-hidden="true" />
                {catalog.faults.length} rejected skill file{catalog.faults.length === 1 ? '' : 's'}
              </div>
              <ul className="mt-1 space-y-1 text-body">
                {catalog.faults.map((fault) => (
                  <li key={`${fault.code}-${fault.skill}`}>
                    <code className="font-code text-label">{fault.code}</code>
                    {fault.skill ? <span className="ml-1 text-label-secondary">{fault.skill}</span> : null}
                    <span className="ml-1 text-muted">— {fault.detail}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </>
      )}
    </section>
  );
}
