'use client';

import React, { useState, useEffect, useCallback } from 'react';
import {
  Search,
  Download,
  Database,
  CheckCircle2,
  AlertCircle,
  HardDrive,
  Cpu,
  Layers,
  Sparkles,
  ExternalLink,
  X,
  RefreshCw,
  Sliders,
  ShieldCheck,
  ChevronDown,
  ChevronRight,
  Eye,
  FileCode,
} from 'lucide-react';
import {
  searchHFModels,
  getHFModelDetails,
  startHFDownload,
  listHFDownloads,
  cancelHFDownload,
} from '@/lib/api';
import { HFModelSummary, HFModelQuantization, HFDownloadTask } from '@/types';

export function HuggingFaceModelHub() {
  const [query, setQuery] = useState('qwen2.5-coder');
  const [library, setLibrary] = useState('gguf');
  const [models, setModels] = useState<HFModelSummary[]>([]);
  const [loading, setLoading] = useState(false);
  const [selectedModel, setSelectedModel] = useState<HFModelSummary | null>(null);
  const [loadingDetails, setLoadingDetails] = useState(false);
  const [downloads, setDownloads] = useState<HFDownloadTask[]>([]);
  const [toastMessage, setToastMessage] = useState<string | null>(null);

  const performSearch = useCallback(async (q = query, lib = library) => {
    setLoading(true);
    try {
      const results = await searchHFModels(q, lib, 16);
      setModels(results);
    } catch (err) {
      console.error('Failed to search Hugging Face:', err);
    } finally {
      setLoading(false);
    }
  }, [query, library]);

  const refreshDownloads = useCallback(async () => {
    try {
      const tasks = await listHFDownloads();
      setDownloads(tasks);
    } catch (err) {
      console.error('Failed to refresh downloads:', err);
    }
  }, []);

  useEffect(() => {
    performSearch('qwen2.5-coder', 'gguf');
    refreshDownloads();
    const interval = setInterval(refreshDownloads, 3000);
    return () => clearInterval(interval);
  }, [performSearch, refreshDownloads]);

  const handleSelectModel = async (model: HFModelSummary) => {
    setSelectedModel(model);
    setLoadingDetails(true);
    try {
      const detailed = await getHFModelDetails(model.id);
      if (detailed) {
        setSelectedModel(detailed);
      }
    } catch (err) {
      console.error('Failed to fetch details:', err);
    } finally {
      setLoadingDetails(false);
    }
  };

  const handleDownload = async (repoId: string, filename: string) => {
    try {
      const task = await startHFDownload(repoId, filename);
      if (task) {
        setToastMessage(`Download started: ${filename}`);
        setTimeout(() => setToastMessage(null), 4000);
        refreshDownloads();
      }
    } catch (err) {
      console.error('Failed to start download:', err);
    }
  };

  const handleCancel = async (taskId: string) => {
    try {
      await cancelHFDownload(taskId);
      refreshDownloads();
    } catch (err) {
      console.error('Failed to cancel download:', err);
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
    <div className="relative rounded-3xl overflow-hidden bg-gradient-to-b from-[#090C13] via-[#0B0E17] to-[#06080D] border border-white/[0.08] shadow-2xl p-7 space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-white/[0.08] pb-5">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-2xl bg-amber-500/10 border border-amber-500/20 backdrop-blur-xl flex items-center justify-center shadow-inner">
            <Database className="w-5 h-5 text-amber-400" />
          </div>
          <div>
            <h2 className="text-base font-semibold text-white tracking-tight flex items-center space-x-2">
              <span>Hugging Face Model Explorer</span>
              <span className="px-2 py-0.5 rounded-full text-[10px] font-mono bg-amber-500/20 text-amber-300 border border-amber-500/30">
                Hub &amp; Downloader
              </span>
            </h2>
            <p className="text-xs text-slate-400 mt-0.5">
              Discover frontier weights, inspect GGUF / MLX quantizations, and stream directly into local air-gapped storage
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-3">
          <div className="flex items-center space-x-1.5 px-3 py-1.5 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-300 text-xs font-mono">
            <ShieldCheck className="w-4 h-4 text-emerald-400" />
            <span>Air-Gap Ready (~/.kubona/models)</span>
          </div>
          <button
            onClick={() => refreshDownloads()}
            className="p-2 rounded-xl bg-white/[0.04] hover:bg-white/[0.08] border border-white/10 text-slate-300 hover:text-white transition-all text-xs flex items-center space-x-1.5"
            title="Refresh Downloads"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Toast Alert */}
      {toastMessage && (
        <div className="p-3 rounded-xl bg-cyan-500/20 border border-cyan-400/40 text-cyan-200 text-xs flex items-center justify-between animate-fade-in shadow-lg">
          <div className="flex items-center space-x-2">
            <Download className="w-4 h-4 text-cyan-400 animate-bounce" />
            <span>{toastMessage}</span>
          </div>
          <button onClick={() => setToastMessage(null)} className="text-cyan-300 hover:text-white">
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      )}

      {/* Active Downloads Panel */}
      {downloads.length > 0 && (
        <div className="p-5 rounded-2xl bg-black/40 border border-cyan-500/30 space-y-3">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-cyan-300 flex items-center space-x-2">
              <Download className="w-4 h-4 text-cyan-400" />
              <span>In-Platform Streaming Downloads ({downloads.length})</span>
            </span>
            <span className="text-[10px] font-mono text-slate-400">Background Stream &amp; SHA-256 Verification</span>
          </div>

          <div className="space-y-2">
            {downloads.map((task) => (
              <div
                key={task.task_id}
                className="p-3 rounded-xl bg-white/[0.03] border border-white/10 flex flex-col md:flex-row items-start md:items-center justify-between gap-3"
              >
                <div className="space-y-1 min-w-0 flex-1">
                  <div className="flex items-center space-x-2">
                    <span className="text-xs font-semibold text-white truncate">{task.filename}</span>
                    <span className="text-[10px] font-mono text-slate-400 truncate">({task.repo_id})</span>
                    <span
                      className={`text-[9px] px-1.5 py-0.5 rounded font-mono font-medium ${
                        task.status === 'completed'
                          ? 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/30'
                          : task.status === 'downloading'
                          ? 'bg-cyan-500/20 text-cyan-300 border border-cyan-500/30 animate-pulse'
                          : task.status === 'failed'
                          ? 'bg-rose-500/20 text-rose-300 border border-rose-500/30'
                          : 'bg-white/10 text-slate-300'
                      }`}
                    >
                      {task.status.toUpperCase()}
                    </span>
                  </div>

                  {/* Progress Bar */}
                  <div className="w-full bg-black/60 rounded-full h-1.5 overflow-hidden border border-white/5">
                    <div
                      className={`h-full transition-all duration-300 ${
                        task.status === 'completed' ? 'bg-emerald-400' : 'bg-cyan-400'
                      }`}
                      style={{ width: `${task.progress_percent}%` }}
                    />
                  </div>

                  <div className="flex items-center justify-between text-[10px] font-mono text-slate-400">
                    <span>
                      {(task.bytes_downloaded / (1024 * 1024)).toFixed(1)} MB
                      {task.total_bytes > 0 && ` / ${(task.total_bytes / (1024 * 1024)).toFixed(1)} MB`} ({task.progress_percent}%)
                    </span>
                    {task.speed_mb_s > 0 && <span>{task.speed_mb_s} MB/s</span>}
                    {task.sha256 && (
                      <span className="text-emerald-400 truncate max-w-xs" title={task.sha256}>
                        SHA-256: {task.sha256.slice(0, 12)}…
                      </span>
                    )}
                  </div>
                </div>

                {task.status === 'downloading' && (
                  <button
                    onClick={() => handleCancel(task.task_id)}
                    className="px-2.5 py-1 rounded-lg bg-rose-500/20 hover:bg-rose-500/30 border border-rose-500/30 text-rose-300 text-xs font-mono shrink-0"
                  >
                    Cancel
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
            <Search className="w-4 h-4 text-slate-400 absolute left-3.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && performSearch(query, library)}
              placeholder="Search Hugging Face models (e.g. qwen2.5-coder, deepseek-r1, llama-3.3, phi-4, llava)..."
              className="w-full pl-10 pr-4 py-2.5 rounded-xl bg-black/40 border border-white/10 focus:border-amber-400/50 text-white text-xs placeholder:text-slate-500 focus:outline-none"
            />
          </div>

          <button
            onClick={() => performSearch(query, library)}
            disabled={loading}
            className="px-4 py-2.5 rounded-xl bg-amber-500/20 hover:bg-amber-500/30 border border-amber-500/30 text-amber-200 text-xs font-semibold flex items-center space-x-1.5 transition-all shadow-md shrink-0"
          >
            {loading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Search className="w-3.5 h-3.5" />}
            <span>Search Hub</span>
          </button>
        </div>

        {/* Quick Filter Chips */}
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-[11px] text-slate-400 font-mono">Curated:</span>
          {quickFilters.map((qf) => (
            <button
              key={qf.label}
              onClick={() => {
                setQuery(qf.q);
                setLibrary(qf.lib);
                performSearch(qf.q, qf.lib);
              }}
              className="px-2.5 py-1 rounded-lg bg-white/[0.04] hover:bg-white/[0.08] border border-white/10 text-slate-300 hover:text-white text-xs font-mono transition-all"
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
            className="p-5 rounded-2xl bg-black/40 border border-white/10 hover:border-amber-500/30 transition-all flex flex-col justify-between space-y-4 group"
          >
            <div className="space-y-2">
              <div className="flex items-start justify-between gap-2">
                <span className="text-xs font-mono text-amber-400 font-semibold truncate">{model.author}</span>
                <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-white/[0.06] text-slate-300 border border-white/10">
                  {model.pipeline_tag || 'text-generation'}
                </span>
              </div>

              <h3 className="text-sm font-semibold text-white group-hover:text-amber-200 transition-colors line-clamp-1">
                {model.id.split('/')[1] || model.id}
              </h3>

              <div className="flex items-center space-x-3 text-[10px] font-mono text-slate-400">
                <span>⬇ {model.downloads.toLocaleString()}</span>
                <span>♥ {model.likes.toLocaleString()}</span>
              </div>

              <div className="flex flex-wrap gap-1 pt-1">
                {model.tags.slice(0, 4).map((t) => (
                  <span key={t} className="text-[9px] font-mono px-1.5 py-0.5 rounded bg-white/[0.04] text-slate-400">
                    {t}
                  </span>
                ))}
              </div>
            </div>

            <div className="pt-2 border-t border-white/[0.06] flex items-center justify-between">
              <button
                onClick={() => handleSelectModel(model)}
                className="px-3 py-1.5 rounded-xl bg-amber-500/20 hover:bg-amber-500/30 border border-amber-500/40 text-amber-200 text-xs font-semibold flex items-center space-x-1.5 transition-all shadow-sm"
              >
                <Layers className="w-3.5 h-3.5 text-amber-400" />
                <span>View Quantizations</span>
              </button>

              <a
                href={`https://huggingface.co/${model.id}`}
                target="_blank"
                rel="noreferrer"
                className="text-slate-400 hover:text-white transition-colors"
                title="View on Hugging Face"
              >
                <ExternalLink className="w-3.5 h-3.5" />
              </a>
            </div>
          </div>
        ))}
      </div>

      {models.length === 0 && !loading && (
        <div className="p-12 text-center text-slate-400 space-y-2 border border-dashed border-white/10 rounded-2xl">
          <Database className="w-8 h-8 mx-auto text-slate-500" />
          <p className="text-xs">No models found on Hugging Face. Try searching for &quot;qwen&quot;, &quot;llama&quot;, or &quot;mlx&quot;.</p>
        </div>
      )}

      {/* Quantizations Inspector Modal */}
      {selectedModel && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-md animate-fade-in">
          <div className="bg-[#0D111A] border border-white/20 rounded-3xl max-w-3xl w-full max-h-[85vh] flex flex-col shadow-2xl overflow-hidden">
            {/* Modal Header */}
            <div className="p-6 border-b border-white/10 flex items-center justify-between">
              <div>
                <span className="text-xs font-mono text-amber-400 font-semibold">{selectedModel.author}</span>
                <h3 className="text-lg font-bold text-white">{selectedModel.id}</h3>
              </div>
              <button
                onClick={() => setSelectedModel(null)}
                className="p-2 rounded-xl bg-white/[0.06] hover:bg-white/[0.1] text-slate-300 hover:text-white transition-all"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            {/* Modal Content */}
            <div className="p-6 overflow-y-auto space-y-4">
              <div className="flex items-center justify-between text-xs text-slate-300">
                <span className="font-semibold text-white">Available Precision &amp; Quantization Files</span>
                <span className="font-mono text-slate-400">Direct HTTP Streaming to ~/.kubona/models</span>
              </div>

              {loadingDetails ? (
                <div className="p-8 text-center text-slate-400 space-y-2">
                  <RefreshCw className="w-6 h-6 mx-auto animate-spin text-amber-400" />
                  <p className="text-xs">Loading quantization files from Hugging Face Hub…</p>
                </div>
              ) : selectedModel.quantizations.length === 0 ? (
                <div className="p-8 text-center text-slate-400 border border-dashed border-white/10 rounded-xl space-y-2">
                  <FileCode className="w-6 h-6 mx-auto text-slate-500" />
                  <p className="text-xs">No single-file .gguf or .mlx weights detected in repository root.</p>
                  <a
                    href={`https://huggingface.co/${selectedModel.id}/tree/main`}
                    target="_blank"
                    rel="noreferrer"
                    className="text-xs text-cyan-400 hover:underline inline-flex items-center space-x-1"
                  >
                    <span>Inspect files on Hugging Face</span>
                    <ExternalLink className="w-3 h-3" />
                  </a>
                </div>
              ) : (
                <div className="space-y-2">
                  {selectedModel.quantizations.map((q) => (
                    <div
                      key={q.filename}
                      className="p-3.5 rounded-xl bg-white/[0.03] border border-white/10 flex items-center justify-between gap-3 hover:bg-white/[0.06] transition-colors"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center space-x-2">
                          <span className="text-xs font-semibold text-white truncate">{q.filename}</span>
                          <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-amber-500/20 text-amber-300 border border-amber-500/30">
                            {q.quant_type}
                          </span>
                        </div>
                        {q.size_bytes && (
                          <span className="text-[10px] font-mono text-slate-400">
                            Size: {(q.size_bytes / (1024 * 1024 * 1024)).toFixed(2)} GB
                          </span>
                        )}
                      </div>

                      <button
                        onClick={() => handleDownload(selectedModel.id, q.filename)}
                        className="px-3.5 py-1.5 rounded-xl bg-cyan-600/30 hover:bg-cyan-600/50 border border-cyan-500/40 text-cyan-200 text-xs font-semibold flex items-center space-x-1.5 transition-all shadow-md shrink-0"
                      >
                        <Download className="w-3.5 h-3.5 text-cyan-400" />
                        <span>Download</span>
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Modal Footer */}
            <div className="p-4 border-t border-white/10 bg-black/40 flex items-center justify-between text-xs text-slate-400">
              <span className="flex items-center space-x-1.5">
                <CheckCircle2 className="w-4 h-4 text-emerald-400" />
                <span>Downloaded weights are immediately available for air-gapped local execution</span>
              </span>
              <button
                onClick={() => setSelectedModel(null)}
                className="px-4 py-1.5 rounded-xl bg-white/[0.08] hover:bg-white/[0.12] text-white text-xs font-semibold transition-all"
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
