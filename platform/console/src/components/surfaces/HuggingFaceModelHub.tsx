'use client';

import React, { useState, useEffect, useCallback, useRef } from 'react';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';
import {
  Search,
  Download,
  Database,
  CheckCircle2,
  
  
  
  Layers,
  
  ExternalLink,
  X,
  RefreshCw,
  
  ShieldCheck,
  
  
  
  FileCode,
} from 'lucide-react';
import {
  searchHFModels,
  getHFModelDetails,
  startHFDownload,
  listHFDownloads,
  cancelHFDownload,
} from '@/lib/api';
import { HFModelSummary,  HFDownloadTask } from '@/types';
import { useFocusTrap } from '@/lib/useFocusTrap';

export function HuggingFaceModelHub() {
  const [query, setQuery] = useState('qwen2.5-coder');
  const [library, setLibrary] = useState('gguf');
  const [models, setModels] = useState<HFModelSummary[]>([]);
  const [loading, setLoading] = useState(false);
  const [selectedModel, setSelectedModel] = useState<HFModelSummary | null>(null);
  const quantPanelRef = useRef<HTMLDivElement | null>(null);
  useFocusTrap(selectedModel !== null, quantPanelRef);
  const [loadingDetails, setLoadingDetails] = useState(false);
  // Honest empty: "No models found" renders only after a search actually
  // ran clean (searched && !searchFailed). A refused search (401/429/5xx —
  // the wrapper throws) renders the refusal + retry, never a fake empty.
  const [searched, setSearched] = useState(false);
  const [searchFailed, setSearchFailed] = useState<string | null>(null);
  const [downloads, setDownloads] = useState<HFDownloadTask[]>([]);
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const [toastTone, setToastTone] = useState<'info' | 'error'>('info');
  // T-11: one click never starts a multi-GB fetch. Known sizes at or
  // above threshold route through an explicit confirm dialog; the server
  // enforces the same bound (confirmed:true) so the dialog is not
  // bypassable via raw POST.
  const [confirmDownload, setConfirmDownload] = useState<{ repoId: string; filename: string; sizeBytes: number } | null>(null);
  // T-11: pending cancel survives the 3 s poll re-render (component-level
  // state, not row-level) and disables the button, so the control stays
  // actionable mid-click instead of vanishing under the cursor.
  const [cancelling, setCancelling] = useState<string | null>(null);

  const showToast = (msg: string, tone: 'info' | 'error' = 'info') => {
    setToastMessage(msg);
    setToastTone(tone);
    setTimeout(() => setToastMessage(null), 5000);
  };

  const performSearch = useCallback(async (q = query, lib = library) => {
    setLoading(true);
    setSearchFailed(null);
    try {
      const results = await searchHFModels(q, lib, 16);
      setModels(Array.isArray(results) ? results : []);
      setSearched(true);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setSearchFailed(msg);
      setSearched(true);
      showToast(`Search failed — nothing returned (${msg}).`, 'error');
    } finally {
      setLoading(false);
    }
  }, [query, library]);

  const refreshDownloads = useCallback(async () => {
    try {
      const tasks = await listHFDownloads();
      setDownloads(tasks);
    } catch (err) {
      showToast(`Download list refused — showing last known (${err instanceof Error ? err.message : String(err)}).`, 'error');
    }
  }, []);

  useEffect(() => {
    let mounted = true;
    let timer: ReturnType<typeof setTimeout> | null = null;
    searchHFModels('qwen2.5-coder', 'gguf', 16)
      .then((results) => {
        if (mounted) {
          setModels(Array.isArray(results) ? results : []);
          setSearched(true);
        }
      })
      .catch((err) => {
        if (mounted) {
          const msg = err instanceof Error ? err.message : String(err);
          setSearchFailed(msg);
          setSearched(true);
          showToast(`Search failed — hub refused (${msg}).`, 'error');
        }
      });

    listHFDownloads()
      .then((tasks) => {
        if (mounted) setDownloads(tasks);
      })
      .catch((err) => {
        if (mounted) showToast(`Download list refused (${err instanceof Error ? err.message : String(err)}).`, 'error');
      });

    // Visibility-gated polling: no background refresh when hidden.
    const poll = () => {
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
        if (mounted) timer = setTimeout(poll, 3000);
        return;
      }
      listHFDownloads()
        .then((tasks) => {
          if (mounted) setDownloads(tasks);
        })
        .catch(() => {
          /* toast would spam — silent on poll, loud on manual refresh */
        })
        .finally(() => {
          if (mounted) timer = setTimeout(poll, 3000);
        });
    };
    timer = setTimeout(poll, 3000);

    return () => {
      mounted = false;
      if (timer) clearTimeout(timer);
    };
  }, []);

  const handleSelectModel = async (model: HFModelSummary) => {
    setSelectedModel(model);
    setLoadingDetails(true);
    try {
      const detailed = await getHFModelDetails(model.id);
      if (detailed) {
        setSelectedModel(detailed);
      } else {
        showToast(`Details refused for ${model.id} — showing summary only.`, 'error');
      }
    } catch (err) {
      showToast(`Failed to fetch details (${err instanceof Error ? err.message : String(err)}).`, 'error');
    } finally {
      setLoadingDetails(false);
    }
  };

  const HF_DOWNLOAD_CONFIRM_BYTES = 100 * 1024 * 1024;

  const handleDownload = async (repoId: string, filename: string, sizeBytes?: number, confirmed = false) => {
    if (!confirmed && typeof sizeBytes === 'number' && Number.isFinite(sizeBytes) && sizeBytes >= HF_DOWNLOAD_CONFIRM_BYTES) {
      setConfirmDownload({ repoId, filename, sizeBytes });
      return;
    }
    setConfirmDownload(null);
    try {
      const task = await startHFDownload(repoId, filename, sizeBytes, confirmed);
      if (task) {
        showToast(`Download started: ${filename}`, 'info');
        refreshDownloads();
      } else {
        showToast(`Download refused for ${filename} — bridge said no. Nothing queued.`, 'error');
      }
    } catch (err) {
      showToast(`Failed to start download (${err instanceof Error ? err.message : String(err)}).`, 'error');
    }
  };

  const handleCancel = async (taskId: string) => {
    if (cancelling !== null) return;
    setCancelling(taskId);
    try {
      const ok = await cancelHFDownload(taskId);
      if (!ok) showToast(`Cancel refused for ${taskId.slice(0, 8)} — task may have finished.`, 'error');
      refreshDownloads();
    } catch (err) {
      showToast(`Failed to cancel download (${err instanceof Error ? err.message : String(err)}).`, 'error');
    } finally {
      setCancelling(null);
    }
  };

  const quickFilters = [
    { label: 'GGUF Models', lib: 'gguf', q: 'qwen2.5-coder' },
    { label: 'Apple MLX', lib: 'mlx', q: 'mlx-community' },
    { label: 'DeepSeek-R1', lib: 'gguf', q: 'DeepSeek-R1-Distill' },
    { label: 'Vision VLMs', lib: 'gguf', q: 'vision' },
    { label: 'Llama 3.3', lib: 'gguf', q: 'Llama-3.3' },
  ];

  return (
    <div className="relative rounded-3xl overflow-hidden bg-[var(--color-page)] border border-[var(--color-border)] shadow-xl p-7 space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-[var(--color-border)] pb-5">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-2xl bg-[var(--color-warning-wash)] border border-[var(--color-warning-edge)] flex items-center justify-center">
            <Database className="w-5 h-5 text-[var(--color-warning-ink)]" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-base font-semibold text-[var(--color-text)] tracking-tight flex items-center space-x-2">
                <span>Hugging Face Model Explorer</span>
                <span className="px-2 py-0.5 rounded-full text-[12px] font-mono bg-[var(--color-warning-wash)] text-[var(--color-warning-ink)] border border-[var(--color-warning-edge)]">
                  Hub &amp; Downloader
                </span>
              </h2>
              <ProvenanceBadge kind="catalog" detail="Model catalog data, not a campaign measurement." />
            </div>
            <p className="text-xs text-[var(--color-muted)] mt-0.5">
              Discover frontier weights, inspect GGUF / MLX quantizations, and stream directly into local air-gapped storage
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-3">
          <div className="flex items-center space-x-1.5 px-3 py-1.5 rounded-xl bg-[var(--color-success-wash)] border border-[var(--color-success-edge)] text-[var(--color-success-ink)] text-xs font-mono">
            <ShieldCheck className="w-4 h-4 text-[var(--color-success-ink)]" />
            <span>Air-Gap Ready (~/.kubona/models)</span>
          </div>
          <button
            onClick={() => refreshDownloads()}
            aria-label="Refresh downloads"
            className="p-2 rounded-xl bg-[var(--color-tile)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[var(--color-label-secondary)] hover:text-[var(--color-text)] transition-all text-xs flex items-center space-x-1.5"
            title="Refresh Downloads"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Toast Alert */}
      {toastMessage && (
        <div className={`p-3 rounded-xl border text-xs flex items-center justify-between animate-fade-in shadow-md ${toastTone === 'error' ? 'bg-[var(--color-danger-wash)] border-[var(--color-danger-edge)] text-[var(--color-danger-ink)]' : 'bg-[var(--color-info-wash)] border-[var(--color-info-edge)] text-[var(--color-info-ink)]'}`}>
          <div className="flex items-center space-x-2">
            <Download className="w-4 h-4 text-[var(--color-info-ink)] animate-bounce" />
            <span>{toastMessage}</span>
          </div>
          <button onClick={() => setToastMessage(null)} aria-label="Dismiss notification" title="Dismiss notification" className="text-[var(--color-info-ink)] hover:text-[var(--color-text)]">
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      )}

      {/* Active Downloads Panel */}
      {downloads.length > 0 && (
        <div className="p-5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-info-edge)] space-y-3">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-[var(--color-info-ink)] flex items-center space-x-2">
              <Download className="w-4 h-4 text-[var(--color-info-ink)]" />
              <span>In-Platform Streaming Downloads ({downloads.length})</span>
            </span>
            <span className="text-[12px] font-mono text-[var(--color-muted)]">Background Stream &amp; SHA-256 Verification</span>
          </div>

          <div className="space-y-2">
            {downloads.map((task) => (
              <div
                key={task.task_id}
                className="p-3 rounded-xl bg-[var(--color-tile)] border border-[var(--color-border)] flex flex-col md:flex-row items-start md:items-center justify-between gap-3"
              >
                <div className="space-y-1 min-w-0 flex-1">
                  <div className="flex items-center space-x-2">
                    <span className="text-xs font-semibold text-[var(--color-text)] truncate">{task.filename}</span>
                    <span className="text-[12px] font-mono text-[var(--color-muted)] truncate">({task.repo_id})</span>
                    <span
                      className={`text-[12px] px-1.5 py-0.5 rounded font-mono font-medium ${
                        task.status === 'completed'
                          ? 'bg-[var(--color-success-wash)] text-[var(--color-success-ink)] border border-[var(--color-success-edge)]'
                          : task.status === 'downloading'
                          ? 'bg-[var(--color-info-wash)] text-[var(--color-info-ink)] border border-[var(--color-info-edge)] animate-pulse'
                          : task.status === 'failed'
                          ? 'bg-[var(--color-danger-wash)] text-[var(--color-danger-ink)] border border-[var(--color-danger-edge)]'
                          : 'bg-[var(--color-fill)] text-[var(--color-label-secondary)]'
                      }`}
                    >
                      {task.status.toUpperCase()}
                    </span>
                  </div>

                  {/* Progress Bar */}
                  <div className="w-full bg-[var(--color-canvas)] rounded-full h-1.5 overflow-hidden border border-[var(--color-border)]">
                    <div
                      className={`h-full transition-all duration-300 ${
                        task.status === 'completed' ? 'bg-[var(--color-success)]' : 'bg-[var(--color-accent)]'
                      }`}
                      style={{ width: `${task.progress_percent}%` }}
                    />
                  </div>

                  <div className="flex items-center justify-between text-[12px] font-mono text-[var(--color-muted)]">
                    <span>
                      {typeof task.bytes_downloaded === 'number' && Number.isFinite(task.bytes_downloaded) ? (task.bytes_downloaded / (1024 * 1024)).toFixed(1) : '—'} MB
                      {typeof task.total_bytes === 'number' && Number.isFinite(task.total_bytes) && task.total_bytes > 0 && ` / ${(task.total_bytes / (1024 * 1024)).toFixed(1)} MB`} ({typeof task.progress_percent === 'number' && Number.isFinite(task.progress_percent) ? task.progress_percent : '—'}%)
                    </span>
                    {typeof task.speed_mb_s === 'number' && Number.isFinite(task.speed_mb_s) && task.speed_mb_s > 0 && <span>{task.speed_mb_s} MB/s</span>}
                    {task.sha256 && (
                      <span className="text-[var(--color-success-ink)] truncate max-w-xs" title={task.sha256}>
                        SHA-256: {task.sha256.slice(0, 12)}…
                      </span>
                    )}
                  </div>
                </div>

                {task.status === 'downloading' && (
                  <button
                    onClick={() => handleCancel(task.task_id)}
                    disabled={cancelling === task.task_id}
                    aria-label={`Cancel download ${task.filename}`}
                    title={`Cancel download ${task.filename}`}
                    className="px-2.5 py-1 rounded-lg bg-[var(--color-danger-wash)] hover:opacity-90 border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-xs font-mono shrink-0 disabled:opacity-50"
                  >
                    {cancelling === task.task_id ? 'Cancelling…' : 'Cancel'}
                  </button>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Search Input & Filter Chips */}
      <div className="space-y-3">
        <div className="flex items-center space-x-2">
          <div className="relative flex-1">
            <Search className="w-4 h-4 text-[var(--color-muted)] absolute left-3.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={query}
              aria-label="Search Hugging Face models"
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && performSearch(query, library)}
              placeholder="Search Hugging Face models (e.g. qwen2.5-coder, deepseek-r1, llama-3.3, phi-4, llava)..."
              className="w-full pl-10 pr-4 py-2.5 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] focus:border-[var(--color-warning-edge)] text-[var(--color-text)] text-xs placeholder:text-[var(--color-faint)] focus:outline-none"
            />
          </div>

          <button
            onClick={() => performSearch(query, library)}
            disabled={loading}
            className="px-4 py-2.5 rounded-xl bg-[var(--color-warning-wash)] hover:opacity-90 border border-[var(--color-warning-edge)] text-[var(--color-warning-ink)] text-xs font-semibold flex items-center space-x-1.5 transition-all shadow-sm shrink-0"
          >
            {loading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Search className="w-3.5 h-3.5" />}
            <span>Search Hub</span>
          </button>
        </div>

        {/* Quick Filter Chips */}
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-[12px] text-[var(--color-muted)] font-mono">Curated:</span>
          {quickFilters.map((qf) => (
            <button
              key={qf.label}
              onClick={() => {
                setQuery(qf.q);
                setLibrary(qf.lib);
                performSearch(qf.q, qf.lib);
              }}
              className="px-2.5 py-1 rounded-lg bg-[var(--color-tile)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[var(--color-label-secondary)] hover:text-[var(--color-text)] text-xs font-mono transition-all"
            >
              {qf.label}
            </button>
          ))}
        </div>
      </div>

      {/* Results Grid & Quantization Inspector Modal */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {models.map((model) => (
          <div
            key={model.id}
            className="p-5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] hover:border-[var(--color-warning-edge)] transition-all flex flex-col justify-between space-y-4 group"
          >
            <div className="space-y-2">
              <div className="flex items-start justify-between gap-2">
                <span className="text-xs font-mono text-[var(--color-warning-ink)] font-semibold truncate">{model.author}</span>
                <span className="text-[12px] font-mono px-2 py-0.5 rounded bg-[var(--color-tile)] text-[var(--color-label-secondary)] border border-[var(--color-border)]">
                  {model.pipeline_tag || 'text-generation'}
                </span>
              </div>

              <h3 className="text-sm font-semibold text-[var(--color-text)] group-hover:text-[var(--color-warning-ink)] transition-colors line-clamp-1">
                {model.id.split('/')[1] || model.id}
              </h3>

              <div className="flex items-center space-x-3 text-[12px] font-mono text-[var(--color-muted)]">
                <span>⬇ {(model.downloads ?? 0).toLocaleString()}</span>
                <span>♥ {(model.likes ?? 0).toLocaleString()}</span>
              </div>

              <div className="flex flex-wrap gap-1 pt-1">
                {(model.tags ?? []).slice(0, 4).map((t) => (
                  <span key={t} className="text-[12px] font-mono px-1.5 py-0.5 rounded bg-[var(--color-tile)] text-[var(--color-muted)]">
                    {t}
                  </span>
                ))}
              </div>
            </div>

            <div className="pt-2 border-t border-[var(--color-border)] flex items-center justify-between">
              <button
                onClick={() => handleSelectModel(model)}
                className="px-3 py-1.5 rounded-xl bg-[var(--color-warning-wash)] hover:opacity-90 border border-[var(--color-warning-edge)] text-[var(--color-warning-ink)] text-xs font-semibold flex items-center space-x-1.5 transition-all shadow-sm"
              >
                <Layers className="w-3.5 h-3.5 text-[var(--color-warning-ink)]" />
                <span>View Quantizations</span>
              </button>

              <a
                href={`https://huggingface.co/${model.id}`}
                target="_blank"
                rel="noreferrer"
                className="text-[var(--color-muted)] hover:text-[var(--color-text)] transition-colors"
                aria-label={`View ${model.id} on Hugging Face`}
                title="View on Hugging Face"
              >
                <ExternalLink className="w-3.5 h-3.5" />
              </a>
            </div>
          </div>
        ))}
      </div>

      {models.length === 0 && !loading && searched && !searchFailed && (
        <div className="p-12 text-center text-[var(--color-muted)] space-y-2 border border-dashed border-[var(--color-border)] rounded-2xl">
          <Database className="w-8 h-8 mx-auto text-[var(--color-faint)]" />
          <p className="text-xs">No models found on Hugging Face. Try searching for &quot;qwen&quot;, &quot;llama&quot;, or &quot;mlx&quot;.</p>
        </div>
      )}

      {models.length === 0 && !loading && searched && searchFailed && (
        <div className="p-12 text-center space-y-3 border border-dashed border-[var(--color-danger-edge)] rounded-2xl bg-[var(--color-danger-wash)]">
          <Database className="w-8 h-8 mx-auto text-[var(--color-danger-ink)]" />
          <p className="text-xs text-[var(--color-danger-ink)]">Hugging Face hub refused the search — nothing returned, nothing assumed ({searchFailed.slice(0, 200)}).</p>
          <button
            type="button"
            onClick={() => void performSearch()}
            className="px-3 py-1.5 rounded-xl bg-[var(--color-tile)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[var(--color-text)] text-xs font-semibold transition"
          >
            Retry search
          </button>
        </div>
      )}

      {/* Quantizations Inspector Modal */}
      {selectedModel && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 animate-fade-in">
          <div
            ref={quantPanelRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby="quant-inspector-title"
            onKeyDown={(e) => {
              if (e.key === 'Escape') setSelectedModel(null);
            }}
            className="bg-[var(--color-float)] border border-[var(--color-border-strong)] rounded-3xl max-w-3xl w-full max-h-[85vh] flex flex-col shadow-2xl overflow-hidden"
          >
            {/* Modal Header */}
            <div className="p-6 border-b border-[var(--color-border)] flex items-center justify-between">
              <div>
                <span className="text-xs font-mono text-[var(--color-warning-ink)] font-semibold">{selectedModel.author}</span>
                <h3 id="quant-inspector-title" className="text-lg font-bold text-[var(--color-text)]">{selectedModel.id}</h3>
              </div>
              <button
                onClick={() => setSelectedModel(null)}
                aria-label="Close quantization inspector"
                title="Close quantization inspector"
                className="p-2 rounded-xl bg-[var(--color-tile)] hover:bg-[var(--color-fill-hover)] text-[var(--color-label-secondary)] hover:text-[var(--color-text)] transition-all"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            {/* Modal Content */}
            <div className="p-6 overflow-y-auto space-y-4">
              <div className="flex items-center justify-between text-xs text-[var(--color-label-secondary)]">
                <span className="font-semibold text-[var(--color-text)]">Available Precision &amp; Quantization Files</span>
                <span className="font-mono text-[var(--color-muted)]">Direct HTTP Streaming to ~/.kubona/models</span>
              </div>

              {loadingDetails ? (
                <div className="p-8 text-center text-[var(--color-muted)] space-y-2">
                  <RefreshCw className="w-6 h-6 mx-auto animate-spin text-[var(--color-warning-ink)]" />
                  <p className="text-xs">Loading quantization files from Hugging Face Hub…</p>
                </div>
              ) : (selectedModel.quantizations ?? []).length === 0 ? (
                <div className="p-8 text-center text-[var(--color-muted)] border border-dashed border-[var(--color-border)] rounded-xl space-y-2">
                  <FileCode className="w-6 h-6 mx-auto text-[var(--color-faint)]" />
                  <p className="text-xs">No single-file .gguf or .mlx weights detected in repository root.</p>
                  <a
                    href={`https://huggingface.co/${selectedModel.id}/tree/main`}
                    target="_blank"
                    rel="noreferrer"
                    className="text-xs text-[var(--color-info-ink)] hover:underline inline-flex items-center space-x-1"
                  >
                    <span>Inspect files on Hugging Face</span>
                    <ExternalLink className="w-3 h-3" />
                  </a>
                </div>
              ) : (
                <div className="space-y-2">
                  {(selectedModel.quantizations ?? []).map((q) => (
                    <div
                      key={q.filename}
                      className="p-3.5 rounded-xl bg-[var(--color-tile)] border border-[var(--color-border)] flex items-center justify-between gap-3 hover:bg-[var(--color-fill-hover)] transition-colors"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center space-x-2">
                          <span className="text-xs font-semibold text-[var(--color-text)] truncate">{q.filename}</span>
                          <span className="text-[12px] font-mono px-2 py-0.5 rounded bg-[var(--color-warning-wash)] text-[var(--color-warning-ink)] border border-[var(--color-warning-edge)]">
                            {q.quant_type}
                          </span>
                        </div>
                        {typeof q.size_bytes === 'number' && Number.isFinite(q.size_bytes) && (
                          <span className="text-[12px] font-mono text-[var(--color-muted)]">
                            Size: {(q.size_bytes / (1024 * 1024 * 1024)).toFixed(2)} GB
                          </span>
                        )}
                      </div>

                      <button
                        onClick={() => handleDownload(selectedModel.id, q.filename, q.size_bytes)}
                        aria-label={`Download ${q.filename}`}
                        title={`Download ${q.filename}`}
                        className="px-3.5 py-1.5 rounded-xl bg-[var(--color-info-wash)] hover:opacity-90 border border-[var(--color-info-edge)] text-[var(--color-info-ink)] text-xs font-semibold flex items-center space-x-1.5 transition-all shadow-sm shrink-0"
                      >
                        <Download className="w-3.5 h-3.5 text-[var(--color-info-ink)]" />
                        <span>Download</span>
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* T-11: size-confirmation dialog. Over-threshold fetches stop
                here — Confirm posts with confirmed:true (the server
                enforces the same bound); Dismiss posts nothing. */}
            {confirmDownload && (
              <div
                role="alertdialog"
                aria-modal="true"
                aria-label={`Confirm download ${confirmDownload.filename}`}
                className="m-4 p-4 rounded-xl bg-[var(--color-warning-wash)] border border-[var(--color-warning-edge)] space-y-2"
              >
                <div className="text-xs font-semibold text-[var(--color-warning-ink)]">
                  Large download — {(confirmDownload.sizeBytes / (1024 * 1024 * 1024)).toFixed(2)} GB
                </div>
                <p className="text-[12px] font-mono text-[var(--color-label-secondary)] break-all">
                  {confirmDownload.filename} ({confirmDownload.repoId})
                </p>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => void handleDownload(confirmDownload.repoId, confirmDownload.filename, confirmDownload.sizeBytes, true)}
                    aria-label={`Confirm download ${confirmDownload.filename}`}
                    className="px-3 py-1.5 rounded-xl bg-[var(--color-accent)] hover:bg-[var(--color-lit)] text-[var(--color-on-accent)] text-xs font-semibold"
                  >
                    Download {(confirmDownload.sizeBytes / (1024 * 1024 * 1024)).toFixed(2)} GB
                  </button>
                  <button
                    type="button"
                    onClick={() => setConfirmDownload(null)}
                    aria-label="Dismiss download confirmation"
                    className="px-3 py-1.5 rounded-xl bg-[var(--color-tile)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[var(--color-label-secondary)] text-xs font-semibold"
                  >
                    Keep looking
                  </button>
                </div>
              </div>
            )}

            {/* Modal Footer */}
            <div className="p-4 border-t border-[var(--color-border)] bg-[var(--color-surface)] flex items-center justify-between text-xs text-[var(--color-muted)]">
              <span className="flex items-center space-x-1.5">
                <CheckCircle2 className="w-4 h-4 text-[var(--color-success-ink)]" />
                <span>Downloaded weights are immediately available for air-gapped local execution</span>
              </span>
              <button
                onClick={() => setSelectedModel(null)}
                className="px-4 py-1.5 rounded-xl bg-[var(--color-tile)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[var(--color-text)] text-xs font-semibold transition-all"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
