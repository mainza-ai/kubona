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
  hasOperatorBearer,
} from '@/lib/api';
import { useAuthPhase, useMultiLabAuthRequired } from '@/domain/auth-state';
import { HFModelSummary,  HFDownloadTask } from '@/types';
import { useFocusTrap } from '@/lib/useFocusTrap';

export function HuggingFaceModelHub() {
  const authPhase = useAuthPhase();
  const multiLabRequired = useMultiLabAuthRequired();
  const deferredAuth =
    multiLabRequired &&
    authPhase !== 'authenticated' &&
    !(typeof hasOperatorBearer === 'function' && hasOperatorBearer());
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
    if (typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent('kubona:surface-toast', { detail: { text: msg, tone } }));
    }
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
    if (deferredAuth) return;
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
  }, [deferredAuth]);

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
    <div className="relative rounded-sheet overflow-hidden bg-page border border-border shadow-xl p-7 space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-border pb-5">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-card bg-warning-wash border border-warning-edge flex items-center justify-center">
            <Database className="w-5 h-5 text-warning-ink" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-body-large font-semibold text-text tracking-tight flex items-center space-x-2">
                <span>Hugging Face Model Explorer</span>
                <span className="px-2 py-0.5 rounded-capsule text-[12px] font-mono bg-warning-wash text-warning-ink border border-warning-edge">
                  Hub &amp; Downloader
                </span>
              </h2>
              <ProvenanceBadge kind="catalog" detail="Model catalog data, not a campaign measurement." />
            </div>
            <p className="text-label text-muted mt-0.5">
              Discover frontier weights, inspect GGUF / MLX quantizations, and stream directly into local air-gapped storage
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-3">
          <div className="flex items-center space-x-1.5 px-3 py-1.5 rounded-row bg-success-wash border border-success-edge text-success-ink text-label font-mono">
            <ShieldCheck className="w-4 h-4 text-success-ink" />
            <span>Air-Gap Ready (~/.kubona/models)</span>
          </div>
          <button
            onClick={() => refreshDownloads()}
            aria-label="Refresh downloads"
            className="p-2 rounded-row bg-tile hover:bg-fill-hover border border-border text-label-secondary hover:text-text transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] text-label flex items-center space-x-1.5"
            title="Refresh Downloads"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Toast Alert */}
      {deferredAuth && (
        <div className="p-3 rounded-row border border-warning-edge bg-warning-wash text-label text-warning-ink">
          Sign in to search HuggingFace models or stream weights in multi-lab mode.
        </div>
      )}

      {/* Active Downloads Panel */}
      {downloads.length > 0 && (
        <div className="p-5 rounded-card bg-surface border border-info-edge space-y-3">
          <div className="flex items-center justify-between">
            <span className="text-label font-semibold text-info-ink flex items-center space-x-2">
              <Download className="w-4 h-4 text-info-ink" />
              <span>In-Platform Streaming Downloads ({downloads.length})</span>
            </span>
            <span className="text-[12px] font-mono text-muted">Background Stream &amp; SHA-256 Verification</span>
          </div>

          <div className="space-y-2">
            {downloads.map((task) => (
              <div
                key={task.task_id}
                className="p-3 rounded-row bg-tile border border-border flex flex-col md:flex-row items-start md:items-center justify-between gap-3"
              >
                <div className="space-y-1 min-w-0 flex-1">
                  <div className="flex items-center space-x-2">
                    <span className="text-label font-semibold text-text truncate">{task.filename}</span>
                    <span className="text-[12px] font-mono text-muted truncate">({task.repo_id})</span>
                    <span
                      className={`text-[12px] px-1.5 py-0.5 rounded-chip font-mono font-medium ${
                        task.status === 'completed'
                          ? 'bg-success-wash text-success-ink border border-success-edge'
                          : task.status === 'downloading'
                          ? 'bg-info-wash text-info-ink border border-info-edge animate-pulse'
                          : task.status === 'failed'
                          ? 'bg-danger-wash text-danger-ink border border-danger-edge'
                          : 'bg-fill text-label-secondary'
                      }`}
                    >
                      {task.status.toUpperCase()}
                    </span>
                  </div>

                  {/* Progress Bar */}
                  <div className="w-full bg-canvas rounded-capsule h-1.5 overflow-hidden border border-border">
                    <div
                      className={`h-full transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] duration-300 ${
                        task.status === 'completed' ? 'bg-success' : 'bg-accent'
                      }`}
                      style={{ width: `${task.progress_percent}%` }}
                    />
                  </div>

                  <div className="flex items-center justify-between text-[12px] font-mono text-muted">
                    <span>
                      {typeof task.bytes_downloaded === 'number' && Number.isFinite(task.bytes_downloaded) ? (task.bytes_downloaded / (1024 * 1024)).toFixed(1) : '—'} MB
                      {typeof task.total_bytes === 'number' && Number.isFinite(task.total_bytes) && task.total_bytes > 0 && ` / ${(task.total_bytes / (1024 * 1024)).toFixed(1)} MB`} ({typeof task.progress_percent === 'number' && Number.isFinite(task.progress_percent) ? task.progress_percent : '—'}%)
                    </span>
                    {typeof task.speed_mb_s === 'number' && Number.isFinite(task.speed_mb_s) && task.speed_mb_s > 0 && <span>{task.speed_mb_s} MB/s</span>}
                    {task.sha256 && (
                      <span className="text-success-ink truncate max-w-xs" title={task.sha256}>
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
                    className="px-2.5 py-1 rounded-control-compact bg-danger-wash hover:opacity-90 border border-danger-edge text-danger-ink text-label font-mono shrink-0 disabled:opacity-50"
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
            <Search className="w-4 h-4 text-muted absolute left-3.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={query}
              aria-label="Search Hugging Face models"
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && performSearch(query, library)}
              placeholder="Search Hugging Face models (e.g. qwen2.5-coder, deepseek-r1, llama-3.3, phi-4, llava)..."
              className="w-full pl-10 pr-4 py-2.5 rounded-row bg-field border border-border focus:border-warning-edge text-text text-label placeholder:text-faint focus:outline-none"
            />
          </div>

          <button
            onClick={() => performSearch(query, library)}
            disabled={loading}
            className="px-4 py-2.5 rounded-row bg-warning-wash hover:opacity-90 border border-warning-edge text-warning-ink text-label font-semibold flex items-center space-x-1.5 transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] shadow-sm shrink-0"
          >
            {loading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Search className="w-3.5 h-3.5" />}
            <span>Search Hub</span>
          </button>
        </div>

        {/* Quick Filter Chips */}
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-[12px] text-muted font-mono">Curated:</span>
          {quickFilters.map((qf) => (
            <button
              key={qf.label}
              onClick={() => {
                setQuery(qf.q);
                setLibrary(qf.lib);
                performSearch(qf.q, qf.lib);
              }}
              className="px-2.5 py-1 rounded-control-compact bg-tile hover:bg-fill-hover border border-border text-label-secondary hover:text-text text-label font-mono transition-[color,background-color,border-color,box-shadow,opacity,transform,filter]"
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
            className="p-5 rounded-card bg-surface border border-border hover:border-warning-edge transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] flex flex-col justify-between space-y-4 group"
          >
            <div className="space-y-2">
              <div className="flex items-start justify-between gap-2">
                <span className="text-label font-mono text-warning-ink font-semibold truncate">{model.author}</span>
                <span className="text-[12px] font-mono px-2 py-0.5 rounded-chip bg-tile text-label-secondary border border-border">
                  {model.pipeline_tag || 'text-generation'}
                </span>
              </div>

              <h3 className="text-body-strong font-semibold text-text group-hover:text-warning-ink transition-colors line-clamp-1">
                {model.id.split('/')[1] || model.id}
              </h3>

              <div className="flex items-center space-x-3 text-[12px] font-mono text-muted">
                <span>⬇ {(model.downloads ?? 0).toLocaleString()}</span>
                <span>♥ {(model.likes ?? 0).toLocaleString()}</span>
              </div>

              <div className="flex flex-wrap gap-1 pt-1">
                {(model.tags ?? []).slice(0, 4).map((t) => (
                  <span key={t} className="text-[12px] font-mono px-1.5 py-0.5 rounded-chip bg-tile text-muted">
                    {t}
                  </span>
                ))}
              </div>
            </div>

            <div className="pt-2 border-t border-border flex items-center justify-between">
              <button
                onClick={() => handleSelectModel(model)}
                className="px-3 py-1.5 rounded-row bg-warning-wash hover:opacity-90 border border-warning-edge text-warning-ink text-label font-semibold flex items-center space-x-1.5 transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] shadow-sm"
              >
                <Layers className="w-3.5 h-3.5 text-warning-ink" />
                <span>View Quantizations</span>
              </button>

              <a
                href={`https://huggingface.co/${model.id}`}
                target="_blank"
                rel="noreferrer"
                className="text-muted hover:text-text transition-colors"
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
        <div className="p-12 text-center text-muted space-y-2 border border-dashed border-border rounded-card">
          <Database className="w-8 h-8 mx-auto text-faint" />
          <p className="text-label">No models found on Hugging Face. Try searching for &quot;qwen&quot;, &quot;llama&quot;, or &quot;mlx&quot;.</p>
        </div>
      )}

      {models.length === 0 && !loading && searched && searchFailed && (
        <div className="p-12 text-center space-y-3 border border-dashed border-danger-edge rounded-card bg-danger-wash">
          <Database className="w-8 h-8 mx-auto text-danger-ink" />
          <p className="text-label text-danger-ink">Hugging Face hub refused the search — nothing returned, nothing assumed ({searchFailed.slice(0, 200)}).</p>
          <button
            type="button"
            onClick={() => void performSearch()}
            className="px-3 py-1.5 rounded-row bg-tile hover:bg-fill-hover border border-border text-text text-label font-semibold transition"
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
            className="bg-float border border-border-strong rounded-sheet max-w-3xl w-full max-h-[85vh] flex flex-col shadow-2xl overflow-hidden"
          >
            {/* Modal Header */}
            <div className="p-6 border-b border-border flex items-center justify-between">
              <div>
                <span className="text-label font-mono text-warning-ink font-semibold">{selectedModel.author}</span>
                <h3 id="quant-inspector-title" className="text-subtitle font-bold text-text">{selectedModel.id}</h3>
              </div>
              <button
                onClick={() => setSelectedModel(null)}
                aria-label="Close quantization inspector"
                title="Close quantization inspector"
                className="p-2 rounded-row bg-tile hover:bg-fill-hover text-label-secondary hover:text-text transition-[color,background-color,border-color,box-shadow,opacity,transform,filter]"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            {/* Modal Content */}
            <div className="p-6 overflow-y-auto space-y-4">
              <div className="flex items-center justify-between text-label text-label-secondary">
                <span className="font-semibold text-text">Available Precision &amp; Quantization Files</span>
                <span className="font-mono text-muted">Direct HTTP Streaming to ~/.kubona/models</span>
              </div>

              {loadingDetails ? (
                <div className="p-8 text-center text-muted space-y-2">
                  <RefreshCw className="w-6 h-6 mx-auto animate-spin text-warning-ink" />
                  <p className="text-label">Loading quantization files from Hugging Face Hub…</p>
                </div>
              ) : (selectedModel.quantizations ?? []).length === 0 ? (
                <div className="p-8 text-center text-muted border border-dashed border-border rounded-row space-y-2">
                  <FileCode className="w-6 h-6 mx-auto text-faint" />
                  <p className="text-label">No single-file .gguf or .mlx weights detected in repository root.</p>
                  <a
                    href={`https://huggingface.co/${selectedModel.id}/tree/main`}
                    target="_blank"
                    rel="noreferrer"
                    className="text-label text-info-ink hover:underline inline-flex items-center space-x-1"
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
                      className="p-3.5 rounded-row bg-tile border border-border flex items-center justify-between gap-3 hover:bg-fill-hover transition-colors"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center space-x-2">
                          <span className="text-label font-semibold text-text truncate">{q.filename}</span>
                          <span className="text-[12px] font-mono px-2 py-0.5 rounded-chip bg-warning-wash text-warning-ink border border-warning-edge">
                            {q.quant_type}
                          </span>
                        </div>
                        {typeof q.size_bytes === 'number' && Number.isFinite(q.size_bytes) && (
                          <span className="text-[12px] font-mono text-muted">
                            Size: {(q.size_bytes / (1024 * 1024 * 1024)).toFixed(2)} GB
                          </span>
                        )}
                      </div>

                      <button
                        onClick={() => handleDownload(selectedModel.id, q.filename, q.size_bytes)}
                        aria-label={`Download ${q.filename}`}
                        title={`Download ${q.filename}`}
                        className="px-3.5 py-1.5 rounded-row bg-info-wash hover:opacity-90 border border-info-edge text-info-ink text-label font-semibold flex items-center space-x-1.5 transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] shadow-sm shrink-0"
                      >
                        <Download className="w-3.5 h-3.5 text-info-ink" />
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
                className="m-4 p-4 rounded-row bg-warning-wash border border-warning-edge space-y-2"
              >
                <div className="text-label font-semibold text-warning-ink">
                  Large download — {(confirmDownload.sizeBytes / (1024 * 1024 * 1024)).toFixed(2)} GB
                </div>
                <p className="text-[12px] font-mono text-label-secondary break-all">
                  {confirmDownload.filename} ({confirmDownload.repoId})
                </p>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => void handleDownload(confirmDownload.repoId, confirmDownload.filename, confirmDownload.sizeBytes, true)}
                    aria-label={`Confirm download ${confirmDownload.filename}`}
                    className="px-3 py-1.5 rounded-row bg-accent hover:bg-lit text-on-accent text-label font-semibold"
                  >
                    Download {(confirmDownload.sizeBytes / (1024 * 1024 * 1024)).toFixed(2)} GB
                  </button>
                  <button
                    type="button"
                    onClick={() => setConfirmDownload(null)}
                    aria-label="Dismiss download confirmation"
                    className="px-3 py-1.5 rounded-row bg-tile hover:bg-fill-hover border border-border text-label-secondary text-label font-semibold"
                  >
                    Keep looking
                  </button>
                </div>
              </div>
            )}

            {/* Modal Footer */}
            <div className="p-4 border-t border-border bg-surface flex items-center justify-between text-label text-muted">
              <span className="flex items-center space-x-1.5">
                <CheckCircle2 className="w-4 h-4 text-success-ink" />
                <span>Downloaded weights are immediately available for air-gapped local execution</span>
              </span>
              <button
                onClick={() => setSelectedModel(null)}
                className="px-4 py-1.5 rounded-row bg-tile hover:bg-fill-hover border border-border text-text text-label font-semibold transition-[color,background-color,border-color,box-shadow,opacity,transform,filter]"
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
