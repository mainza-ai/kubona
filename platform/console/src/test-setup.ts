import { afterEach } from 'vitest';
import { resetSharedResourcesForTests } from '@/domain/shared-resource';
import { resetTranscriptStoresForTests } from '@/domain/transcript-store';

afterEach(() => {
  resetSharedResourcesForTests();
  resetTranscriptStoresForTests();
});
