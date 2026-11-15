'use client';

import { useSyncExternalStore } from 'react';
import type { WorkflowPlanDraft } from '@/types';

/**
 * C8 — the latest dry-run plan, published by the workflow planner and
 * projected on the canvas. Ephemeral by design: the plan is a draft that has
 * not executed, so it is not server state. Creating tasks attaches their ids
 * (idempotently) so the card can flip to "Open in Tasks".
 */

let latest: WorkflowPlanDraft | null = null;
const listeners = new Set<() => void>();

function emit(): void {
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function getSnapshot(): WorkflowPlanDraft | null {
  return latest;
}

function getServerSnapshot(): WorkflowPlanDraft | null {
  return null;
}

export function publishWorkflowPlan(draft: Omit<WorkflowPlanDraft, 'createdAt'> & { createdAt?: number }): void {
  latest = { ...draft, createdAt: draft.createdAt ?? Date.now() };
  emit();
}

export function setWorkflowPlanTaskIds(taskIds: string[]): void {
  if (!latest) return;
  latest = { ...latest, taskIds };
  emit();
}

export function getWorkflowPlan(): WorkflowPlanDraft | null {
  return latest;
}

export function clearWorkflowPlan(): void {
  latest = null;
  emit();
}

export function useWorkflowPlan(): WorkflowPlanDraft | null {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}

export function resetWorkflowPlanForTests(): void {
  latest = null;
  listeners.clear();
}
