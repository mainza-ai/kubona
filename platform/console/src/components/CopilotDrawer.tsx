'use client';

import React, { useState, useEffect, useRef,  useMemo } from 'react';
import {
  Sparkles,
  Atom,
  Scale,
  Cpu,
  BookOpen,
  CheckCircle2,
  ChevronRight,
  ChevronDown,
  ChevronUp,
  Bot,
  Maximize2,
  Minimize2,
  PanelRightClose,
  Copy,
  Check,
  Paperclip,
  ArrowUp,
  Search,
  Download,
  X,
  Trash2,
  RotateCcw,
  ThumbsUp,
  ThumbsDown,
  Pencil,
} from 'lucide-react';
import { ChatMessage, CampaignState, RunPhase, Proposal } from '@/types';
import { fetchMessages, sendChatMessage, streamChatMessage, createSession, runTribunal, fetchCampaignState, launchProposal, clearMessages, regenerateChat, sendFeedback, rewriteChat, fetchJobs, jobStatusLabel, isTerminalJobStatus } from '@/lib/api';
import { MathText } from './common/MathText';
import { Tip } from './common/Tip';
import { MemoryPanel } from './MemoryPanel';
import type { CanvasWidgetKind } from './sovereign/WidgetStage';

interface CopilotDrawerProps {
  sessionId: string;
  campaign?: CampaignState | null;
  isExpanded?: boolean;
  onToggleExpand?: () => void;
  onExperimentLaunched?: () => void;
  onCampaignUpdated?: (campaign: CampaignState) => void;
  onSelectSurface?: (surface: string) => void;
  onMaterialize?: (kind: CanvasWidgetKind, title: string, jobId?: string) => void;
  onRunStateChange?: (phase: RunPhase, detail: string) => void;
  onClose?: () => void;
}

export function CopilotDrawer({
  sessionId,
  campaign,
  isExpanded: externalExpanded,
  onToggleExpand,
  onExperimentLaunched,
  onCampaignUpdated,
  onSelectSurface,
  onMaterialize,
  onRunStateChange,
  onClose,
}: CopilotDrawerProps) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [elapsedSeconds, setElapsedSeconds] = useState(0);
  const [runningTribunal, setRunningTribunal] = useState(false);
  const [tribunalStage, setTribunalStage] = useState<string>('Convening multi-agent scientific tribunal...');
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [internalExpanded, setInternalExpanded] = useState(false);
  const isExpanded = externalExpanded !== undefined ? externalExpanded : internalExpanded;
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [expandedTurns, setExpandedTurns] = useState<Record<string, boolean>>({});
  const [launchingId, setLaunchingId] = useState<string | null>(null);
  const [launchedNote, setLaunchedNote] = useState<Record<string, string>>({});
  const [launchedJobs, setLaunchedJobs] = useState<Record<string, string>>({});
  const [streamingId, setStreamingId] = useState<string | null>(null);
  const [infoMessage, setInfoMessage] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const idCounter = useRef(0);
  const nextTempId = (prefix: string) => `${prefix}-${++idCounter.current}`;
  // Attachments ride the next direct send as explicit context, then clear.
  const [attachments, setAttachments] = useState<Array<{ name: string; text: string }>>([]);
  const [clearing, setClearing] = useState(false);
  const [regenerating, setRegenerating] = useState(false);
  const [votes, setVotes] = useState<Record<string, 'up' | 'down'>>({});
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editDraft, setEditDraft] = useState('');
  const [rewriting, setRewriting] = useState(false);
  // In-chat search: filter-narrow + jump between matches (no KaTeX surgery).
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchIndex, setSearchIndex] = useState(0);
  const messageRefs = useRef<Record<string, HTMLDivElement | null>>({});

  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const attachInputRef = useRef<HTMLInputElement>(null);
  const timerRef = useRef<NodeJS.Timeout | null>(null);

  const handleToggleExpand = () => {
    if (onToggleExpand) {
      onToggleExpand();
    } else {
      setInternalExpanded((prev) => !prev);
    }
  };

  const scrollToBottom = () => {
    if (messagesContainerRef.current) {
      messagesContainerRef.current.scrollTo({
        top: messagesContainerRef.current.scrollHeight,
        behavior: 'smooth',
      });
    }
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages, loading, runningTribunal]);

  useEffect(() => {
    if (sessionId) {
      fetchMessages(sessionId).then((msgs) => {
        if (msgs && msgs.length > 0) {
          setMessages(msgs);
        }
      });
    }
  }, [sessionId]);

  useEffect(() => {
    if (loading || runningTribunal) {
      const initTimer = setTimeout(() => setElapsedSeconds(0), 0);
      timerRef.current = setInterval(() => {
        setElapsedSeconds((prev) => prev + 1);
      }, 1000);
      return () => {
        clearTimeout(initTimer);
        if (timerRef.current) clearInterval(timerRef.current);
      };
    }
  }, [loading, runningTribunal]);

  const getActiveSessionId = async (): Promise<string> => {
    if (sessionId) return sessionId;
    const newSess = await createSession('Quantum Research Campaign #1', 150.0);
    return newSess ? newSess.id : '';
  };

  const handleSend = async (customPrompt?: string) => {
    const rawText = customPrompt || input.trim();
    if (!rawText || loading || runningTribunal) return;

    // Slash commands never reach the model.
    if (rawText.startsWith('/')) {
      handleSlash(rawText);
      if (!customPrompt) setInput('');
      return;
    }

    // Attachments ride this send as explicit context, then clear.
    const attached = attachments.map((a) => `[attached ${a.name}]\n${a.text}`).join('\n');
    const textToSend = [attached, rawText].filter(Boolean).join('\n');
    if (attachments.length > 0) setAttachments([]);

    if (!customPrompt) setInput('');
    setErrorMessage(null);
    setInfoMessage(null);

    const activeSessionId = await getActiveSessionId();

    const tempUserMsg: ChatMessage = {
      id: nextTempId(`temp-user`),
      session_id: activeSessionId,
      role: 'user',
      content: textToSend,
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, tempUserMsg]);
    setLoading(true);
    onRunStateChange?.('thinking', textToSend);

    // Live bubble: tokens stream into it; the done event replaces it with
    // the stored (sanitized) records. Unary fallback on stream failure.
    const liveId = nextTempId(`live-assistant`);
    setMessages((prev) => [
      ...prev,
      {
        id: liveId,
        session_id: activeSessionId,
        role: 'assistant',
        content: '',
        created_at: new Date().toISOString(),
      },
    ]);
    setStreamingId(liveId);
    const controller = new AbortController();
    abortRef.current = controller;

    const settleStream = (done: { user: ChatMessage; assistant: ChatMessage; proposal?: unknown; campaign?: CampaignState; duration_ms?: number; usage?: Record<string, number> }) => {
      setMessages((prev) => {
        const filtered = prev.filter((m) => m.id !== tempUserMsg.id && m.id !== liveId);
        const assistant: ChatMessage = { ...done.assistant };
        if (!assistant.proposal_json && done.proposal) {
          assistant.proposal_json = done.proposal as Proposal;
        }
        if (assistant.duration_ms == null && done.duration_ms != null) {
          assistant.duration_ms = done.duration_ms;
        }
        if (!assistant.usage && done.usage) {
          assistant.usage = {
            completion_tokens: done.usage.completion_tokens,
            prompt_tokens: done.usage.prompt_tokens,
            total_tokens: done.usage.total_tokens,
          };
        }
        return [...filtered, done.user, assistant];
      });
      if (done.campaign && onCampaignUpdated) {
        onCampaignUpdated(done.campaign);
      } else if (onCampaignUpdated) {
        fetchCampaignState(activeSessionId).then((c) => {
          if (c) onCampaignUpdated(c);
        });
      }
      if (onExperimentLaunched) onExperimentLaunched();
    };

    try {
      const done = await streamChatMessage(activeSessionId, textToSend, (tok) => {
        setMessages((prev) =>
          prev.map((m) => (m.id === liveId ? { ...m, content: m.content + tok } : m))
        );
      }, controller.signal);
      settleStream(done);
    } catch (streamErr) {
      if (streamErr instanceof DOMException && streamErr.name === 'AbortError') {
        // Operator stop: drop the partial bubble but keep the sent prompt.
        // The server run may still complete and appear on next history load.
        setMessages((prev) => prev.filter((m) => m.id !== liveId));
        setInfoMessage('Stopped. The backend run may still finish — its result will appear in history.');
        setTimeout(() => setInfoMessage(null), 6000);
        setLoading(false);
        setStreamingId(null);
        abortRef.current = null;
        onRunStateChange?.('idle', '');
        return;
      }
      console.warn('Stream failed, falling back to unary chat:', streamErr);
      try {
        const res = await sendChatMessage(activeSessionId, textToSend);
        if (res) {
          setMessages((prev) => {
            const filtered = prev.filter((m) => m.id !== tempUserMsg.id && m.id !== liveId);
            return [...filtered, res.user, res.assistant];
          });
          if (onCampaignUpdated) {
            fetchCampaignState(activeSessionId).then((c) => {
              if (c) onCampaignUpdated(c);
            });
          }
          if (onExperimentLaunched) onExperimentLaunched();
        } else {
          setMessages((prev) => prev.filter((m) => m.id !== liveId));
          setErrorMessage('AI engine did not respond. Check the model in Sovereign settings (dock → mesh → settings).');
        }
      } catch (err: unknown) {
        setMessages((prev) => prev.filter((m) => m.id !== liveId));
        setErrorMessage(err instanceof Error ? err.message : 'Connection error to backend.');
      }
    } finally {
      setLoading(false);
      setStreamingId(null);
      onRunStateChange?.('idle', '');
    }
  };

  const handleRunTribunal = async (topic: string) => {
    if (runningTribunal || loading) return;
    setRunningTribunal(true);
    setTribunalStage('Ansatz Scout: Proposing low-depth quantum circuit parameterization...');
    onRunStateChange?.('tribunal', 'Ansatz Scout: proposing parameterization…');
    setErrorMessage(null);

    const activeSessionId = await getActiveSessionId();

    const tempUserMsg: ChatMessage = {
      id: nextTempId(`temp-tribunal`),
      session_id: activeSessionId,
      role: 'user',
      content: `Convene Research Tribunal for: ${topic}`,
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, tempUserMsg]);

    // Live turn polling: tribunal turns land as messages mid-run (backend
    // onTurn records each one). Every new turn updates the canvas run-state
    // so the matching probe ignites — no simulated stages. Stateless:
    // anything created after the prompt that isn't the prompt counts.
    const runStartEpoch = new Date(tempUserMsg.created_at).getTime() - 1000;
    const turnPoll = setInterval(async () => {
      try {
        const fresh = await fetchMessages(activeSessionId);
        const arrived = fresh.filter(
          (m) => m.role !== 'user' && new Date(m.created_at).getTime() >= runStartEpoch
        );
        const lastTurn = arrived[arrived.length - 1];
        if (lastTurn) {
          const roleLabel = String(lastTurn.role).replace(/_/g, ' ');
          setTribunalStage(`${lastTurn.role}: turn recorded (${arrived.length} so far)…`);
          onRunStateChange?.('tribunal', `${roleLabel} turn recorded`);
          // Merge arrived turns into the visible transcript live.
          setMessages((prev) => {
            const ids = new Set(prev.map((m) => m.id));
            const missing = arrived.filter((m) => !ids.has(m.id));
            return missing.length > 0 ? [...prev, ...missing] : prev;
          });
        }
      } catch {
        /* transient poll failure — next round retries */
      }
    }, 2500);

    try {
      const res = await runTribunal(activeSessionId, topic);
      if (res && res.assistant_message) {
        const assistantWithTribunal: ChatMessage = {
          ...res.assistant_message,
          tribunal: res.tribunal,
        };

        setMessages((prev) => {
          const filtered = prev.filter((m) => m.id !== tempUserMsg.id);
          return [...filtered, tempUserMsg, assistantWithTribunal];
        });

        if (res.campaign && onCampaignUpdated) {
          onCampaignUpdated(res.campaign);
        } else if (onCampaignUpdated) {
          fetchCampaignState(activeSessionId).then((c) => {
            if (c) onCampaignUpdated(c);
          });
        }
        if (onExperimentLaunched) onExperimentLaunched();
      } else {
        setErrorMessage('Failed to convene tribunal. Check the model in Sovereign settings and the bridge status in Fleet Radar.');
      }
    } catch (err: unknown) {
      setErrorMessage(err instanceof Error ? err.message : 'Tribunal connection error.');
    } finally {
      clearInterval(turnPoll);
      setRunningTribunal(false);
      onRunStateChange?.('idle', '');
    }
  };

  const handleLaunchLast = async () => {
    for (let i = messages.length - 1; i >= 0; i--) {
      const m = messages[i];
      if (m.role !== 'user' && m.proposal_json) {
        await handleLaunchProposal(m.id, m.proposal_json);
        return;
      }
    }
    setInfoMessage('No proposal in this conversation yet — ask for a simulation first.');
    setTimeout(() => setInfoMessage(null), 4000);
  };

  const handleSlash = (raw: string) => {
    const [cmd, ...rest] = raw.slice(1).split(/\s+/);
    const arg = rest.join(' ').trim();
    if (cmd === 'tribunal') {
      handleRunTribunal(arg || `${targetSystem} ground state electronic structure`);
    } else if (cmd === 'launch') {
      handleLaunchLast();
    } else if (cmd === 'help') {
      setInfoMessage('Commands: /tribunal <topic> · /launch (most recent proposal) · /help');
      setTimeout(() => setInfoMessage(null), 5000);
    } else {
      setInfoMessage(`Unknown command /${cmd}. Try /help.`);
      setTimeout(() => setInfoMessage(null), 4000);
    }
  };

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  const handleAttachFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    try {
      const text = await file.text();
      const trimmed = text.length > 4000 ? text.slice(0, 4000) + '\n…[truncated]' : text;
      setAttachments((prev) => [...prev.filter((a) => a.name !== file.name), { name: file.name, text: trimmed }]);
    } catch {
      setErrorMessage(`Could not read ${file.name}: binary or unreadable file.`);
    }
  };

  const removeAttachment = (name: string) => {
    setAttachments((prev) => prev.filter((a) => a.name !== name));
  };

  // In-chat search matches (message ids in order).
  const matchedIds = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return [];
    return messages.filter((m) => m.content.toLowerCase().includes(q)).map((m) => m.id);
  }, [messages, searchQuery]);

  const jumpToMatch = (dir: 1 | -1) => {
    if (matchedIds.length === 0) return;
    const next = (searchIndex + dir + matchedIds.length) % matchedIds.length;
    setSearchIndex(next);
    requestAnimationFrame(() => {
      messageRefs.current[matchedIds[next]]?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    });
  };

  // Slash commands are defined after the handlers they invoke (no TDZ).

  const exportTranscript = () => {
    const lines = [
      `# Kubona co-pilot transcript`,
      ``,
      `- Session: ${sessionId || 'unsaved'}`,
      `- Target: ${targetDisplayName}`,
      `- Exported: ${new Date().toISOString()}`,
      ``,
    ];
    for (const m of messages) {
      const when = new Date(m.created_at).toLocaleString();
      lines.push(`## ${m.role} · ${when}${m.model ? ` · ${m.model}` : ''}`);
      lines.push(``);
      lines.push(m.content);
      lines.push(``);
      if (m.proposal_json) {
        lines.push('```json');
        lines.push(JSON.stringify(m.proposal_json, null, 2));
        lines.push('```');
        lines.push(``);
      }
    }
    const blob = new Blob([lines.join('\n')], { type: 'text/markdown' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `kubona-transcript-${(sessionId || 'session').slice(0, 8)}.md`;
    a.click();
    URL.revokeObjectURL(url);
  };

  // Regenerate is available when the transcript ends in an assistant turn.
  const canRegenerate =
    messages.length > 0 && messages[messages.length - 1].role !== 'user';

  const handleClearChat = async () => {
    if (clearing || loading || runningTribunal || messages.length === 0) return;
    const activeSessionId = await getActiveSessionId();
    if (!activeSessionId) return;
    if (!confirm('Clear this conversation? Messages are deleted; the session, campaign, and ledger history survive.')) return;
    setClearing(true);
    try {
      const deleted = await clearMessages(activeSessionId);
      if (deleted != null) {
        setMessages([]);
        setVotes({});
        setInfoMessage(`Cleared ${deleted} message${deleted === 1 ? '' : 's'}.`);
        setTimeout(() => setInfoMessage(null), 4000);
      } else {
        setErrorMessage('Clear refused by backend — nothing was deleted.');
      }
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Clear failed.');
    } finally {
      setClearing(false);
    }
  };

  const handleRegenerate = async () => {
    if (regenerating || loading || runningTribunal || !canRegenerate) return;
    const activeSessionId = await getActiveSessionId();
    if (!activeSessionId) return;
    setRegenerating(true);
    setErrorMessage(null);
    onRunStateChange?.('thinking', 'regenerating last answer');
    try {
      const res = await regenerateChat(activeSessionId);
      if (res?.assistant) {
        setMessages((prev) => {
          // Drop trailing assistant turns the server already removed, then
          // append the fresh answer (dedupe by id for safety).
          const trimmed: ChatMessage[] = [...prev];
          while (trimmed.length > 0 && trimmed[trimmed.length - 1].role !== 'user') {
            trimmed.pop();
          }
          if (!trimmed.some((m) => m.id === res.assistant.id)) {
            trimmed.push(res.assistant);
          }
          return trimmed;
        });
        if (onExperimentLaunched) onExperimentLaunched();
      } else {
        setErrorMessage('Regenerate refused — no user message to answer, or backend error.');
      }
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Regenerate failed.');
    } finally {
      setRegenerating(false);
      onRunStateChange?.('idle', '');
    }
  };
  const handleVote = async (messageId: string, vote: 'up' | 'down') => {
    const activeSessionId = await getActiveSessionId();
    if (!activeSessionId) return;
    setVotes((prev) => ({ ...prev, [messageId]: vote }));
    const ok = await sendFeedback(activeSessionId, messageId, vote);
    if (!ok) {
      setVotes((prev) => {
        const next = { ...prev };
        delete next[messageId];
        return next;
      });
      setErrorMessage('Feedback not recorded — backend refused.');
    }
  };

  // Edit-resend branching: rewrite the user turn server-side (later turns
  // deleted), truncate the local transcript at the branch point, then
  // regenerate the answer for the edited prompt.
  const handleRewriteSave = async (messageId: string) => {
    const draft = editDraft.trim();
    if (!draft || rewriting || loading || runningTribunal) return;
    const activeSessionId = await getActiveSessionId();
    if (!activeSessionId) return;
    setRewriting(true);
    setErrorMessage(null);
    try {
      const rewritten = await rewriteChat(activeSessionId, messageId, draft);
      if (!rewritten) {
        setErrorMessage('Rewrite refused — only your own messages can be edited.');
        return;
      }
      setEditingId(null);
      setMessages((prev) => {
        const idx = prev.findIndex((m) => m.id === messageId);
        const cut = idx === -1 ? prev : prev.slice(0, idx);
        return [...cut, rewritten];
      });
      onRunStateChange?.('thinking', 'answering edited prompt');
      const res = await regenerateChat(activeSessionId);
      if (res?.assistant) {
        setMessages((prev) => {
          if (prev.some((m) => m.id === res.assistant.id)) return prev;
          return [...prev, res.assistant];
        });
        if (onExperimentLaunched) onExperimentLaunched();
      } else {
        setErrorMessage('Regenerate after rewrite refused — branch is clean, retry manually.');
      }
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Rewrite failed.');
    } finally {
      setRewriting(false);
      onRunStateChange?.('idle', '');
    }
  };

  const toggleTurnExpand = (turnKey: string) => {
    setExpandedTurns((prev) => ({
      ...prev,
      [turnKey]: !prev[turnKey],
    }));
  };

  const handleLaunchProposal = async (messageId: string, proposal: Proposal) => {
    if (launchingId) return;
    const activeSessionId = await getActiveSessionId();
    if (!activeSessionId) return;
    setLaunchingId(messageId);
    try {
      const res = await launchProposal(activeSessionId, proposal);
      if (res && 'job' in res && res.job) {
        const jobId = String(res.job.id);
        setLaunchedJobs((prev) => ({ ...prev, [messageId]: jobId }));
        setLaunchedNote((prev) => ({ ...prev, [messageId]: `Launched: job ${jobId.slice(0, 8)} · tracking…` }));
        if (onExperimentLaunched) onExperimentLaunched();
        // Inline job tracking: poll session jobs for this id and report the
        // live status into the card. Bounded (8 rounds); terminal states stop
        // early. Status labels + failure reasons come from the backend.
        let rounds = 0;
        const track = async () => {
          rounds += 1;
          const jobs = await fetchJobs(activeSessionId);
          const hit = jobs.find((j) => String(j.id) === jobId);
          if (hit?.status != null) {
            const label = jobStatusLabel(hit.status);
            // Failure reasons: backend writes `error` (failJob/cancel paths);
            // the success proto also carries `error_message` (empty on success).
            const result = (hit as { result_json?: { error?: string; error_message?: string } }).result_json;
            const reasonText = result?.error || result?.error_message || '';
            const reason = reasonText ? ` · ${String(reasonText).slice(0, 160)}` : '';
            setLaunchedNote((prev) => ({
              ...prev,
              [messageId]: `Job ${jobId.slice(0, 8)} · ${label}${hit.completed_at ? ` · done ${new Date(hit.completed_at).toLocaleTimeString()}` : ''}${reason}`,
            }));
            if (isTerminalJobStatus(hit.status) || rounds >= 8) return;
          } else if (rounds >= 8) {
            setLaunchedNote((prev) => ({ ...prev, [messageId]: `Launched: job ${jobId.slice(0, 8)} · status unavailable` }));
            return;
          }
          setTimeout(track, 3000);
        };
        setTimeout(track, 3000);
      } else if (res && 'error' in res) {
        setLaunchedNote((prev) => ({ ...prev, [messageId]: `Launch refused: ${res.error}` }));
      } else {
        setLaunchedNote((prev) => ({ ...prev, [messageId]: 'Launch refused by backend — nothing dispatched.' }));
      }
    } catch {
      setLaunchedNote((prev) => ({ ...prev, [messageId]: 'Launch failed — nothing dispatched.' }));
    } finally {
      setLaunchingId(null);
    }
  };

  const renderProposalCard = (m: ChatMessage) => {
    if (!m.proposal_json) return null;
    const p = m.proposal_json;
    const note = launchedNote[m.id];
    const jobId = launchedJobs[m.id];
    return (
      <div className="w-full max-w-[95%] rounded-2xl bg-black/40 border border-cyan-400/25 p-3 space-y-2">
        <div className="flex items-center gap-1.5 text-[10px] font-code text-cyan-400">
          <span>✦</span>
          <span>LAUNCHABLE PROPOSAL</span>
          {p.designed_by === 'server-fallback' && (
            <span className="ml-auto px-1.5 py-0.5 rounded bg-amber-400/15 border border-amber-400/40 text-amber-300" title="Circuit synthesized by the server as scaffolding, not designed by the model — review before launch">
              SERVER FALLBACK
            </span>
          )}
        </div>
        <div className="text-xs text-white font-medium">{p.target}</div>
        <div className="grid grid-cols-2 gap-2 text-[10px] font-code text-slate-400">
          <div><span className="text-white/40 block">Tier</span><span className="text-cyan-200">{p.recommended_tier}</span></div>
          <div><span className="text-white/40 block">Spend</span><span className="text-amber-300">{p.spend_estimate}</span></div>
        </div>
        <div className="text-[10px] font-code text-slate-500 truncate" title={p.classical_anchor}>
          anchor · {p.classical_anchor}
        </div>
        {note ? (
          jobId ? (
            <button
              onClick={() => onMaterialize?.('job', `Job ${jobId.slice(0, 8)} · detail`, jobId)}
              title="Open full job detail (params, counts, result)"
              className="w-full text-left text-[11px] font-code text-emerald-300 hover:text-emerald-200 hover:bg-white/5 rounded-lg px-1.5 py-1 transition-all"
            >
              {note} →
            </button>
          ) : (
            <div className="text-[11px] font-code text-emerald-300">{note}</div>
          )
        ) : (
          <button
            onClick={() => handleLaunchProposal(m.id, p)}
            disabled={launchingId === m.id}
            className="w-full py-1.5 px-3 rounded-xl bg-cyan-500/15 hover:bg-cyan-500/25 border border-cyan-400/30 text-cyan-200 text-[11px] font-semibold transition-all disabled:opacity-50"
          >
            {launchingId === m.id ? 'Dispatching…' : 'Launch on local simulator'}
          </button>
        )}
      </div>
    );
  };

  const targetSystem = campaign?.target_system || 'H2';

  // Plain function (not memoized): trivial string mapping, and the compiler
  // rejects the optional-chaining dep form.
  const displayNameFor = (rawTarget: string): string => {
    const raw = rawTarget || 'H2';
    if (raw.toUpperCase() === 'H2' || raw === 'H₂') return 'Hydrogen Molecule (H₂)';
    if (raw.toUpperCase().includes('LIH')) return 'Lithium Hydride (LiH)';
    if (raw.toUpperCase().includes('BEH2')) return 'Beryllium Hydride (BeH₂)';
    if (raw.toUpperCase().includes('H2O')) return 'Water Molecule (H₂O)';
    if (raw.toUpperCase().includes('N2')) return 'Dinitrogen (N₂)';
    if (raw.toUpperCase().includes('CO')) return 'Carbon Monoxide (CO)';
    return raw;
  };
  const targetDisplayName = displayNameFor(campaign?.target_system || 'H2');
  const discrepancy = campaign?.classical_anchor?.discrepancy_mha;
  const isChemicalAccuracyMet = discrepancy != null && discrepancy <= 1.6;

  // Contextual follow-ups derived from the latest assistant turn.
  // Data-only specs computed inline (tiny list — no memo needed), so no
  // memoized closures over ref-touching handlers. Click dispatch happens in
  // runFollowUp (event-handler context, refs allowed).
  type FollowUpSpec = { label: string; accent: string; key: string };
  const lastAssistantForFollowUps = [...messages].reverse().find((m) => m.role !== 'user');
  const followUps: FollowUpSpec[] = lastAssistantForFollowUps?.proposal_json
    ? [
        { label: 'Launch it', accent: 'text-cyan-400', key: 'launch-last' },
        { label: 'Inspect circuit', accent: 'text-cyan-400', key: 'show-circuit' },
        { label: 'Tribunal review', accent: 'text-emerald-400', key: 'tribunal-topic' },
      ]
    : lastAssistantForFollowUps?.tribunal
      ? [
          { label: 'Inspect circuit', accent: 'text-cyan-400', key: 'show-circuit' },
          { label: 'Shadow estimates', accent: 'text-cyan-400', key: 'show-shadows' },
          { label: 'Active space', accent: 'text-amber-400', key: 'show-molecule' },
        ]
      : [
          { label: 'H₂ Bond PES', accent: 'text-amber-400', key: 'send-pes' },
          { label: `${targetSystem} Tribunal`, accent: 'text-emerald-400', key: 'tribunal-topic' },
          { label: 'Canary Fidelity', accent: 'text-cyan-400', key: 'send-canary' },
          { label: 'Classical FCI Anchor', accent: 'text-amber-400', key: 'send-anchor' },
        ];

  const runFollowUp = (key: string) => {
    if (key === 'launch-last') {
      handleLaunchLast();
    } else if (key === 'show-circuit') {
      onMaterialize?.('circuit', 'Circuit · live');
    } else if (key === 'show-shadows') {
      onMaterialize?.('shadows', 'Shadow estimates');
    } else if (key === 'show-molecule') {
      onMaterialize?.('molecule', 'Active space');
    } else if (key === 'tribunal-topic') {
      handleRunTribunal(`${targetDisplayName} equilibrium state`);
    } else if (key === 'send-pes') {
      handleSend('Calculate potential energy surface for H2');
    } else if (key === 'send-canary') {
      handleSend('Simulate Clifford RB canary on IBM Heron');
    } else if (key === 'send-anchor') {
      handleSend('Verify classical PySCF STO-3G ground truth');
    }
  };

  return (
    <div
      className="flex flex-col h-full w-full bg-[#0A0C10]/95 backdrop-blur-2xl text-slate-100 select-none font-sovereign overflow-hidden"
    >
      {/* Header: Kubona Copilot identity */}
      <div className="px-5 py-4 border-b border-white/5 flex items-center justify-between shrink-0">
        <div className="flex items-center gap-2.5">
          <div className="w-2.5 h-2.5 rounded-full bg-cyan-400 animate-pulse" />
          <div>
            <div className="text-xs font-semibold text-white tracking-wide flex items-center gap-1.5">
              <span>KUBONA COPILOT</span>
              <span className="px-1.5 py-0.5 rounded bg-cyan-400/10 text-cyan-400 font-code text-[9px]">ACTIVE</span>
            </div>
            <div className="text-[10px] text-slate-400">
              Target: {targetDisplayName} · Δ = {discrepancy != null ? `${discrepancy.toFixed(2)} mHa` : 'UNVERIFIED'}
            </div>
          </div>
        </div>

        <div className="flex items-center space-x-1.5">
          {/* In-chat search */}
          <Tip label="Search conversation" sub="Filter + jump between matches" position="bottom">
          <button
            onClick={() => setSearchOpen((v) => !v)}
            aria-label="Search this conversation"
            className={`p-1.5 rounded-lg transition-colors focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none ${
              searchOpen ? 'text-cyan-300 bg-white/10' : 'text-slate-400 hover:text-white hover:bg-white/10'
            }`}
          >
            <Search className="w-3.5 h-3.5" />
          </button>
          </Tip>

          {/* Export transcript */}
          <Tip label="Export transcript" sub="Markdown download" position="bottom">
          <button
            onClick={exportTranscript}
            aria-label="Export transcript"
            disabled={messages.length === 0}
            className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-colors disabled:opacity-30 focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
          >
            <Download className="w-3.5 h-3.5" />
          </button>
          </Tip>

          {/* Regenerate last answer */}
          <Tip label="Regenerate answer" sub="Re-answers the last prompt" position="bottom">
          <button
            onClick={handleRegenerate}
            aria-label="Regenerate last answer"
            disabled={regenerating || loading || runningTribunal || !canRegenerate}
            className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-colors disabled:opacity-30 focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
          >
            <RotateCcw className={`w-3.5 h-3.5 ${regenerating ? 'animate-spin' : ''}`} />
          </button>
          </Tip>

          {/* Clear chat */}
          <Tip label="Clear chat" sub="Deletes messages; session survives" position="bottom">
          <button
            onClick={handleClearChat}
            aria-label="Clear chat history"
            disabled={clearing || loading || runningTribunal || messages.length === 0}
            className="p-1.5 rounded-lg text-slate-400 hover:text-rose-300 hover:bg-rose-500/10 transition-colors disabled:opacity-30 focus-visible:ring-2 focus-visible:ring-rose-400 focus-visible:outline-none"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
          </Tip>

          {/* Spatial Expand/Contract Button */}
          <Tip label={isExpanded ? 'Dock co-pilot' : 'Expand co-pilot'} position="bottom">
          <button
            onClick={handleToggleExpand}
            aria-label={isExpanded ? 'Contract to Side Dock' : 'Expand to Spatial Salon'}
            className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-colors focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
          >
            {isExpanded ? <Minimize2 className="w-3.5 h-3.5" /> : <Maximize2 className="w-3.5 h-3.5 text-cyan-400" />}
          </button>
          </Tip>

          {/* Close Companion Button */}
          {onClose && (
            <Tip label="Close co-pilot" sub="⌘J to reopen" position="bottom">
            <button
              onClick={onClose}
              aria-label="Close Copilot (⌘J)"
              className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-colors focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
            >
              <PanelRightClose className="w-3.5 h-3.5" />
            </button>
            </Tip>
          )}
        </div>
      </div>

      {/* Active Scientific Focus Ribbon */}
      <div className="mx-5 mt-3 px-3 py-2 rounded-xl bg-black/40 border border-white/[0.06] backdrop-blur-xl flex items-center justify-between text-[11px] font-code shrink-0">
        <div className="flex items-center space-x-2 text-slate-300">
          <span className="text-cyan-400 font-bold">TARGET:</span>
          <span className="truncate max-w-[180px]">{targetDisplayName}</span>
        </div>
        <div className="flex items-center space-x-1.5">
          <span className={`w-1.5 h-1.5 rounded-full ${isChemicalAccuracyMet ? 'bg-emerald-400' : 'bg-amber-400'}`} />
          <span className={isChemicalAccuracyMet ? 'text-emerald-300' : 'text-amber-300'}>
            Δ = {discrepancy != null ? `${discrepancy.toFixed(2)} mHa` : 'UNVERIFIED'}
          </span>
        </div>
      </div>

      {/* In-chat search bar */}
      {searchOpen && (
        <div className="mx-5 mt-2 flex items-center gap-1.5 px-2.5 py-1.5 rounded-xl bg-black/40 border border-white/10 shrink-0">
          <Search className="w-3.5 h-3.5 text-slate-500 shrink-0" />
          <input
            autoFocus
            value={searchQuery}
            onChange={(e) => {
              setSearchQuery(e.target.value);
              setSearchIndex(0);
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') jumpToMatch(1);
              if (e.key === 'Escape') setSearchOpen(false);
            }}
            placeholder="Find in conversation…"
            className="flex-1 min-w-0 bg-transparent text-xs text-white placeholder-slate-500 focus:outline-none"
          />
          <span className="text-[10px] font-code text-slate-500 shrink-0">
            {matchedIds.length === 0 ? '0' : `${searchIndex + 1}/${matchedIds.length}`}
          </span>
          <Tip label="Previous match" position="top">
          <button onClick={() => jumpToMatch(-1)} aria-label="Previous match" className="p-0.5 text-slate-400 hover:text-white">
            <ChevronUp className="w-3.5 h-3.5" />
          </button>
          </Tip>
          <Tip label="Next match" position="top">
          <button onClick={() => jumpToMatch(1)} aria-label="Next match" className="p-0.5 text-slate-400 hover:text-white">
            <ChevronDown className="w-3.5 h-3.5" />
          </button>
          </Tip>
          <Tip label="Close search" sub="Esc" position="top">
          <button onClick={() => { setSearchOpen(false); setSearchQuery(''); }} aria-label="Close search" className="p-0.5 text-slate-400 hover:text-white">
            <X className="w-3.5 h-3.5" />
          </button>
          </Tip>
        </div>
      )}

      {/* Messages Feed */}
      <div
        ref={messagesContainerRef}
        className="flex-1 overflow-y-auto space-y-4 p-4 scrollbar-thin"
      >
        {messages.length === 0 && (
          <div className="p-3 rounded-xl bg-white/[0.02] border border-white/5 text-[11px] text-slate-400 leading-relaxed">
            <span className="text-white font-medium">Autonomous Swarm Initialized:</span> Exploring {targetDisplayName}. Type your intent or ask for telemetry breakdown on demand.
          </div>
        )}
        {messages.length === 0 && (
          <div className="space-y-2">

            {/* Quick Scientific Suggestion Cards */}
            <div className="pt-1 space-y-2 text-left">
              <button
                onClick={() => handleSend('Map H2 ground state potential energy surface across nuclear coordinates')}
                className="w-full p-3 rounded-2xl bg-black/40 hover:bg-black/70 border border-white/10 hover:border-cyan-500/40 text-xs text-slate-200 transition-all flex items-center justify-between group shadow-sm"
              >
                <div className="flex items-center space-x-2.5">
                  <span className="text-amber-400 text-[10px]">✦</span>
                  <span className="font-medium">H₂ Potential Energy Surface</span>
                </div>
                <ChevronRight className="w-3.5 h-3.5 text-slate-400 group-hover:text-cyan-300 group-hover:translate-x-0.5 transition-all" />
              </button>

              <button
                onClick={() => handleRunTribunal('Lithium Hydride (LiH) ground state electronic structure')}
                className="w-full p-3 rounded-2xl bg-black/40 hover:bg-black/70 border border-white/10 hover:border-emerald-500/40 text-xs text-slate-200 transition-all flex items-center justify-between group shadow-sm"
              >
                <div className="flex items-center space-x-2.5">
                  <span className="text-emerald-400 text-[10px]">✦</span>
                  <span className="font-medium">Convene Tribunal for LiH</span>
                </div>
                <ChevronRight className="w-3.5 h-3.5 text-slate-400 group-hover:text-emerald-300 group-hover:translate-x-0.5 transition-all" />
              </button>

              <button
                onClick={() => handleSend('Verify our variational ansatz on the IBM Heron QPU noise model with Clifford canary')}
                className="w-full p-3 rounded-2xl bg-black/40 hover:bg-black/70 border border-white/10 hover:border-cyan-500/40 text-xs text-slate-200 transition-all flex items-center justify-between group shadow-sm"
              >
                <div className="flex items-center space-x-2.5">
                  <span className="text-cyan-400 text-[10px]">✦</span>
                  <span className="font-medium">Probe IBM Heron Noise Model</span>
                </div>
                <ChevronRight className="w-3.5 h-3.5 text-slate-400 group-hover:text-cyan-300 group-hover:translate-x-0.5 transition-all" />
              </button>
            </div>
          </div>
        )}

        {/* Render Conversation Stream */}
        {messages.map((m) => {
          const isUser = m.role === 'user';
          const hasTribunal = Boolean(m.tribunal && m.tribunal.turns && m.tribunal.turns.length > 0);

          return (
            <div
              key={m.id}
              ref={(el) => {
                messageRefs.current[m.id] = el;
              }}
              className={`flex flex-col space-y-1.5 rounded-xl -mx-1 px-1 py-0.5 transition-shadow ${
                isUser ? 'items-end' : 'items-start'
              } ${
                matchedIds.length > 0 && matchedIds[searchIndex] === m.id
                  ? 'ring-1 ring-cyan-400/50'
                  : ''
              }`}
            >
              {/* Message Meta Info + actions */}
              <div className="flex items-center space-x-2 text-[10px] text-slate-400 font-mono px-1">
                <span>{isUser ? 'You' : 'Kubona Tribunal Salon'}</span>
                <span>•</span>
                <span>{new Date(m.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                {!isUser && m.model && (
                  <>
                    <span>•</span>
                    <span className="text-slate-500">served by {m.model}</span>
                  </>
                )}
                {!isUser && m.duration_ms != null && (
                  <>
                    <span>•</span>
                    <span className="text-slate-500">{(m.duration_ms / 1000).toFixed(1)}s</span>
                  </>
                )}
                {!isUser && m.usage?.completion_tokens != null && (
                  <>
                    <span>•</span>
                    <span className="text-slate-500">{m.usage.completion_tokens} tok</span>
                  </>
                )}
                <Tip label="Copy message" position="top">
                <button
                  onClick={() => copyToClipboard(m.content, `copy-${m.id}`)}
                  aria-label="Copy message"
                  className="p-0.5 rounded text-slate-600 hover:text-slate-200 transition-colors focus-visible:ring-1 focus-visible:ring-cyan-400 focus-visible:outline-none"
                >
                  {copiedId === `copy-${m.id}` ? <Check className="w-3 h-3 text-emerald-400" /> : <Copy className="w-3 h-3" />}
                </button>
                </Tip>
                {isUser && !streamingId && (
                  <Tip label="Edit and resend" sub="Branches the thread" position="top">
                  <button
                    onClick={() => {
                      setEditingId(m.id);
                      setEditDraft(m.content);
                    }}
                    aria-label="Edit and resend"
                    className="p-0.5 rounded text-slate-600 hover:text-slate-200 transition-colors focus-visible:ring-1 focus-visible:ring-cyan-400 focus-visible:outline-none"
                  >
                    <Pencil className="w-3 h-3" />
                  </button>
                  </Tip>
                )}
                {!isUser && (
                  <>
                    <Tip label="Helpful" sub="Feeds the credibility flywheel" position="top">
                    <button
                      onClick={() => handleVote(m.id, 'up')}
                      aria-label="Mark helpful"
                      aria-pressed={votes[m.id] === 'up'}
                      className={`p-0.5 rounded transition-colors focus-visible:ring-1 focus-visible:ring-cyan-400 focus-visible:outline-none ${
                        votes[m.id] === 'up' ? 'text-emerald-400' : 'text-slate-600 hover:text-slate-200'
                      }`}
                    >
                      <ThumbsUp className="w-3 h-3" />
                    </button>
                    </Tip>
                    <Tip label="Not helpful" sub="Feeds the credibility flywheel" position="top">
                    <button
                      onClick={() => handleVote(m.id, 'down')}
                      aria-label="Mark not helpful"
                      aria-pressed={votes[m.id] === 'down'}
                      className={`p-0.5 rounded transition-colors focus-visible:ring-1 focus-visible:ring-cyan-400 focus-visible:outline-none ${
                        votes[m.id] === 'down' ? 'text-rose-400' : 'text-slate-600 hover:text-slate-200'
                      }`}
                    >
                      <ThumbsDown className="w-3 h-3" />
                    </button>
                    </Tip>
                  </>
                )}
              </div>

              {/* Standard Message Bubble (or inline editor when branching) */}
              {editingId === m.id && isUser ? (
                <div className="w-full max-w-[92%] rounded-2xl bg-black/40 border border-cyan-400/30 p-2.5 space-y-2 self-end">
                  <textarea
                    autoFocus
                    value={editDraft}
                    onChange={(e) => setEditDraft(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) handleRewriteSave(m.id);
                      if (e.key === 'Escape') setEditingId(null);
                    }}
                    rows={3}
                    maxLength={4000}
                    className="w-full bg-transparent text-xs text-white placeholder-slate-500 focus:outline-none resize-y"
                  />
                  <div className="flex items-center justify-end gap-1.5">
                    <button
                      onClick={() => setEditingId(null)}
                      disabled={rewriting}
                      className="px-2.5 py-1 rounded-lg bg-white/5 hover:bg-white/10 text-slate-300 text-[11px] transition-all disabled:opacity-40"
                    >
                      Cancel
                    </button>
                    <button
                      onClick={() => handleRewriteSave(m.id)}
                      disabled={rewriting || !editDraft.trim()}
                      className="px-2.5 py-1 rounded-lg bg-cyan-500/15 hover:bg-cyan-500/25 border border-cyan-400/30 text-cyan-200 text-[11px] font-semibold transition-all disabled:opacity-40"
                    >
                      {rewriting ? 'Resending…' : 'Resend (branches thread)'}
                    </button>
                  </div>
                </div>
              ) : (
              !hasTribunal && (
                <div
                  className={`px-3.5 py-2.5 text-xs leading-relaxed max-w-[92%] ${
                    isUser
                      ? 'rounded-2xl rounded-tr-sm bg-cyan-500/15 border border-cyan-400/30 text-white'
                      : 'rounded-2xl rounded-tl-sm bg-[#13161F] border border-white/10 text-slate-200 shadow-lg'
                  }`}
                >
                  <MathText text={m.content} />
                  {m.id === streamingId && <span className="text-cyan-400 animate-pulse"> ▍</span>}
                </div>
              )
              )}
              {!isUser && renderProposalCard(m)}

              {/* Multi-Agent Tribunal Salon Presentation */}
              {hasTribunal && m.tribunal && (
                <div className="w-full space-y-2">
                  <div className="flex items-center gap-1.5 text-[10px] font-code text-cyan-400 px-1">
                    <Bot className="w-3 h-3" />
                    <span>TRIBUNAL SYNTHESIS</span>
                  </div>
                  <div className="w-full rounded-2xl rounded-tl-sm bg-[#13161F] border border-white/10 p-3.5 space-y-4 shadow-lg">
                  {/* Tribunal Header Banner */}
                  <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
                    <div className="flex items-center space-x-2.5">
                      <div className="w-7 h-7 rounded-xl bg-indigo-500/20 border border-indigo-500/30 flex items-center justify-center">
                        <Scale className="w-3.5 h-3.5 text-indigo-400" />
                      </div>
                      <div>
                        <div className="font-bold text-white text-xs">Autonomous Tribunal Deliberation</div>
                        <div className="text-[10px] text-slate-400 font-mono">Target: {m.tribunal.target || m.tribunal.molecule}</div>
                      </div>
                    </div>

                    <span
                      className={`px-2.5 py-0.5 rounded-full text-[10px] font-mono font-bold border flex items-center space-x-1.5 ${
                        m.tribunal.consensus_passed
                          ? 'bg-emerald-950/70 border-emerald-500/40 text-emerald-300'
                          : 'bg-amber-950/70 border-amber-500/40 text-amber-300'
                      }`}
                    >
                      <CheckCircle2 className="w-3 h-3" />
                      <span>{m.tribunal.consensus_passed ? 'CONSENSUS VERIFIED' : 'DELIBERATING'}</span>
                    </span>
                  </div>

                  {/* 4 Colleague Discourse Cards */}
                  <div className="space-y-2.5">
                    {m.tribunal.turns.map((turn, idx) => {
                      const turnKey = `${m.id}-turn-${idx}`;
                      const isTurnExpanded = expandedTurns[turnKey] !== false; // Default expanded

                      let accentColor = 'border-cyan-500/40 text-cyan-300';
                      let roleIcon = Atom;
                      let roleTitle = 'Ansatz Scout';

                      if (turn.role.includes('disconfirmation') || turn.name.includes('Disconfirmation')) {
                        accentColor = 'border-amber-500/40 text-amber-300';
                        roleIcon = Scale;
                        roleTitle = 'Disconfirmation Officer';
                      } else if (turn.role.includes('auditor') || turn.name.includes('Auditor')) {
                        accentColor = 'border-indigo-500/40 text-indigo-300';
                        roleIcon = Cpu;
                        roleTitle = 'Device Auditor';
                      } else if (turn.role.includes('scribe') || turn.name.includes('Scribe')) {
                        accentColor = 'border-emerald-500/40 text-emerald-300';
                        roleIcon = BookOpen;
                        roleTitle = 'Epistemic Scribe';
                      }

                      const RoleIconComponent = roleIcon;

                      return (
                        <div
                          key={idx}
                          className="rounded-2xl bg-white/[0.02] border border-white/[0.06] p-3.5 space-y-2 hover:border-white/15 transition-all"
                        >
                          {/* Colleague Header */}
                          <div
                            onClick={() => toggleTurnExpand(turnKey)}
                            className="flex items-center justify-between cursor-pointer"
                          >
                            <div className="flex items-center space-x-2">
                              <RoleIconComponent className={`w-3.5 h-3.5 ${accentColor}`} />
                              <span className="font-bold text-white text-xs tracking-tight">{turn.name || roleTitle}</span>
                              <span className={`text-[9px] font-mono uppercase px-1.5 py-0.2 rounded border bg-black/40 ${accentColor}`}>
                                {turn.verdict || 'ANALYZED'}
                              </span>
                            </div>

                            <Tip label={isTurnExpanded ? 'Collapse turn' : 'Expand turn'} position="left">
                            <button aria-label={isTurnExpanded ? 'Collapse turn' : 'Expand turn'} className="text-slate-400 hover:text-white transition-colors">
                              {isTurnExpanded ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
                            </button>
                            </Tip>
                          </div>

                          {/* Colleague Content */}
                          {isTurnExpanded && (
                            <div className="text-xs text-slate-300 leading-relaxed font-sans pt-1 border-t border-white/[0.04]">
                              <MathText text={turn.content} />
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>

                  {/* Consensus Synthesis */}
                  {m.tribunal.synthesis && (
                    <div className="p-3 rounded-2xl bg-[#13161F] border border-white/10 space-y-2">
                      <div className="flex items-center space-x-1.5 text-xs font-bold text-emerald-300">
                        <Sparkles className="w-3.5 h-3.5" />
                        <span>Tribunal Synthesis &amp; Proposed Action</span>
                      </div>
                      <div className="text-xs text-slate-200 leading-relaxed font-sans">
                        <MathText text={m.tribunal.synthesis} />
                      </div>

                      {/* Interactive Canvas & Notebook Artifact Buttons */}
                      <div className="pt-2 flex flex-wrap items-center gap-2">
                        {onSelectSurface && (
                          <button
                            onClick={() => onSelectSurface('surface1')}
                            className="px-3 py-1.5 rounded-xl bg-cyan-600/30 hover:bg-cyan-600/50 border border-cyan-500/40 text-cyan-200 text-[11px] font-semibold flex items-center space-x-1.5 transition-all shadow-sm"
                          >
                            <Atom className="w-3.5 h-3.5" />
                            <span>View on Living Canvas</span>
                          </button>
                        )}
                        {onSelectSurface && (
                          <button
                            onClick={() => onSelectSurface('surface7')}
                            className="px-3 py-1.5 rounded-xl bg-indigo-600/30 hover:bg-indigo-600/50 border border-indigo-500/40 text-indigo-200 text-[11px] font-semibold flex items-center space-x-1.5 transition-all shadow-sm"
                          >
                            <BookOpen className="w-3.5 h-3.5" />
                            <span>Pin to Living Paper</span>
                          </button>
                        )}
                        <button
                          onClick={() => copyToClipboard(m.tribunal?.synthesis || '', m.id)}
                          className="px-3 py-1.5 rounded-xl bg-white/[0.05] hover:bg-white/10 border border-white/10 text-slate-300 text-[11px] font-semibold flex items-center space-x-1.5 transition-all"
                        >
                          {copiedId === m.id ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                          <span>{copiedId === m.id ? 'Copied' : 'Copy Synthesis'}</span>
                        </button>
                      </div>

                      {/* On-demand canvas dispatch: agents show, don't tell */}
                      {onMaterialize && (
                        <div className="pt-1 flex flex-wrap items-center gap-1.5">
                          <button
                            onClick={() => onMaterialize('circuit', 'Circuit · live')}
                            className="px-2.5 py-1 rounded-full bg-white/5 hover:bg-white/10 border border-white/10 text-[10px] text-slate-300 transition-all flex items-center gap-1"
                          >
                            <span className="text-cyan-400">✦</span> Inspect circuit
                          </button>
                          <button
                            onClick={() => onMaterialize('shadows', 'Shadow estimates')}
                            className="px-2.5 py-1 rounded-full bg-white/5 hover:bg-white/10 border border-white/10 text-[10px] text-slate-300 transition-all flex items-center gap-1"
                          >
                            <span className="text-cyan-400">✦</span> Shadow estimates
                          </button>
                          <button
                            onClick={() => onMaterialize('molecule', 'Active space')}
                            className="px-2.5 py-1 rounded-full bg-white/5 hover:bg-white/10 border border-white/10 text-[10px] text-slate-300 transition-all flex items-center gap-1"
                          >
                            <span className="text-amber-400">✦</span> Active space
                          </button>
                          <button
                            onClick={() => onMaterialize('mandate', 'Spend mandate')}
                            className="px-2.5 py-1 rounded-full bg-white/5 hover:bg-white/10 border border-white/10 text-[10px] text-slate-300 transition-all flex items-center gap-1"
                          >
                            <span className="text-amber-400">✦</span> Spend card
                          </button>
                        </div>
                      )}
                    </div>
                  )}
                  </div>
              {!isUser && renderProposalCard(m)}
                </div>
              )}
            </div>
          );
        })}

        {/* Live Loading & Multi-Stage Reasoning Aura */}
        {(loading || runningTribunal) && (
          <div className="p-4 rounded-3xl bg-black/60 border border-cyan-500/30 space-y-2.5 animate-pulse shadow-xl shadow-cyan-950/30">
            <div className="flex items-center justify-between text-xs font-mono text-cyan-300">
              <div className="flex items-center space-x-2">
                <Sparkles className="w-4 h-4 animate-spin text-cyan-400" />
                <span className="font-bold">Autonomous Scientific Reasoning</span>
              </div>
              <span>{elapsedSeconds}s</span>
            </div>

            <p className="text-xs text-slate-300 leading-relaxed font-sans">
              {runningTribunal ? tribunalStage : `Reasoning with the configured model…`}
            </p>
          </div>
        )}

        {errorMessage && (
          <div className="p-3.5 rounded-2xl bg-red-950/40 border border-red-800/40 text-red-300 text-xs font-sans">
            {errorMessage}
          </div>
        )}

        {infoMessage && (
          <div className="p-3.5 rounded-2xl bg-white/[0.04] border border-white/10 text-slate-300 text-xs font-sans">
            {infoMessage}
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Input Stage with Suggestion Pills */}
      <div className="shrink-0 p-4 border-t border-white/5 bg-[#0A0C10] space-y-2">
        {/* Contextual follow-ups */}
        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 scrollbar-none text-xs">
          {followUps.map((f) => (
            <button
              key={f.label}
              onClick={() => runFollowUp(f.key)}
              className="px-2.5 py-1 rounded-full bg-white/5 hover:bg-white/10 border border-white/10 text-[10px] text-slate-300 transition-all flex items-center gap-1 whitespace-nowrap"
            >
              <span className={f.accent}>✦</span> {f.label}
            </button>
          ))}
        </div>

        {/* Session memory inspector (tenant-scoped recall + retain) */}
        <MemoryPanel sessionId={sessionId} />

        {/* Composer Card */}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            handleSend();
          }}
          className="relative bg-[#13161F] border border-white/10 rounded-2xl p-2 focus-within:border-cyan-400/60 focus-within:ring-1 focus-within:ring-cyan-400/40 transition-all shadow-inner"
        >
          {/* Attachment pills (explicit context riding the next send) */}
          {attachments.length > 0 && (
            <div className="flex flex-wrap gap-1.5 px-2 pb-1.5">
              {attachments.map((a) => (
                <span key={a.name} className="flex items-center gap-1 px-2 py-0.5 rounded-full bg-cyan-500/10 border border-cyan-400/30 text-[10px] font-code text-cyan-200">
                  <Paperclip className="w-3 h-3" />
                  {a.name}
                  <button type="button" onClick={() => removeAttachment(a.name)} aria-label={`Remove ${a.name}`} className="hover:text-white">
                    <X className="w-3 h-3" />
                  </button>
                </span>
              ))}
            </div>
          )}
          {/* Slash hint */}
          {input.startsWith('/') && (
            <div className="px-2 pb-1.5 text-[10px] font-code text-slate-500">
              <span className="text-cyan-300">/tribunal</span> convene · <span className="text-cyan-300">/launch</span> last proposal · <span className="text-cyan-300">/help</span>
            </div>
          )}
          <textarea
            rows={2}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                handleSend();
              }
            }}
            placeholder="Command swarm, query proofs, or adjust target..."
            className="w-full bg-transparent text-xs text-white placeholder-slate-500 focus:outline-none resize-none px-2 py-1 font-sovereign"
          />
          <input
            ref={attachInputRef}
            type="file"
            accept=".pdb,.cif,.xyz,.mol,.sdf,.txt,.md"
            className="hidden"
            onChange={handleAttachFile}
          />
          <div className="flex items-center justify-between pt-1 border-t border-white/5 px-1">
            <div className="flex items-center gap-1">
              <Tip label="Attach file" sub="PDB · CIF · XYZ · text" position="top">
              <button
                type="button"
                onClick={() => attachInputRef.current?.click()}
                aria-label="Attach molecular file (PDB/CIF/XYZ)"
                className="p-1 rounded-lg text-slate-400 hover:text-white hover:bg-white/5 transition-all focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
              >
                <Paperclip className="w-4 h-4" />
              </button>
              </Tip>
              <span className="text-[10px] font-code text-slate-600 ml-1">⌘K</span>
            </div>
            <Tip label={streamingId ? 'Stop generation' : 'Send directive'} position="top">
            <button
              type={streamingId ? 'button' : 'submit'}
              onClick={streamingId ? () => abortRef.current?.abort() : undefined}
              disabled={!streamingId && (!input.trim() || loading || runningTribunal)}
              aria-label={streamingId ? 'Stop generation' : 'Send directive'}
              className={`w-7 h-7 rounded-xl flex items-center justify-center transition-all focus-visible:ring-2 focus-visible:ring-cyan-200 focus-visible:outline-none ${
                streamingId
                  ? 'bg-rose-500 hover:bg-rose-400 text-white shadow-[0_0_15px_rgba(244,63,94,0.35)]'
                  : 'bg-cyan-400 hover:bg-cyan-300 text-black shadow-[0_0_15px_rgba(0,229,255,0.3)] disabled:opacity-40'
              }`}
            >
              {streamingId ? (
                <span className="w-3 h-3 rounded-[2px] bg-current" />
              ) : (
                <ArrowUp className="w-4 h-4 font-bold" />
              )}
            </button>
            </Tip>
          </div>
        </form>

        {/* Tribunal Action Button */}
        <button
          type="button"
          onClick={() => handleRunTribunal(input.trim() || `${targetSystem} ground state electronic structure`)}
          disabled={loading || runningTribunal}
          className="w-full py-2 px-3 rounded-2xl bg-white/5 hover:bg-white/10 border border-white/10 text-xs text-slate-200 font-semibold flex items-center justify-center space-x-2 transition-all disabled:opacity-40 active:scale-[0.99]"
        >
          <Sparkles className="w-3.5 h-3.5 text-cyan-400" />
          <span>Convene Multi-Agent Research Tribunal</span>
        </button>
        <div className="text-center">
          <span className="text-[9px] text-slate-600 font-code">Autonomous AI Swarm · Local-first</span>
        </div>
      </div>
    </div>
  );
}
