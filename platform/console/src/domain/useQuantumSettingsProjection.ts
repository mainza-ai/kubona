'use client';

import { fetchQuantumBackends, fetchQuantumSettings, type QuantumBackendInfo } from '@/lib/api';
import { ResourceRefusal } from './resource-store';
import { useSharedResource } from './shared-resource';

export type QuantumSettingsProjection = {
  backends: QuantumBackendInfo[];
  settings: Record<string, unknown> | null;
};

async function loadQuantumSettings(): Promise<QuantumSettingsProjection> {
  const [backends, settings] = await Promise.all([fetchQuantumBackends(), fetchQuantumSettings()]);
  if (settings === null) throw new ResourceRefusal('quantum provider settings unavailable');
  return { backends, settings };
}

/** One ref-counted owner for the quantum provider catalog and saved selection. */
export function useQuantumSettingsProjection() {
  return useSharedResource<QuantumSettingsProjection>('quantum-settings', loadQuantumSettings, {
    intervalMs: 60_000,
  });
}
