import { afterEach } from 'vitest';
import { resetSharedResourcesForTests } from '@/domain/shared-resource';

afterEach(() => {
  resetSharedResourcesForTests();
});
