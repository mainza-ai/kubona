import { afterEach, vi } from 'vitest';
import { resetSharedResourcesForTests } from '@/domain/shared-resource';
import { resetTranscriptStoresForTests } from '@/domain/transcript-store';

// Keep component tests synchronous; production still crosses the lazy boundary.
vi.mock('@/components/data/LazyChart', () => import('@/components/data/DataChart').then((module) => ({ Chart: module.Chart })));

afterEach(() => {
  resetSharedResourcesForTests();
  resetTranscriptStoresForTests();
});
