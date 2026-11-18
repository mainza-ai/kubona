// Publication domain boundary: manuscript lifecycle transport used by the
// Results Studio publication surface.
export {
  buildManuscript,
  createManuscript,
  createManuscriptAnnotation,
  downloadManuscriptArtifact,
  fetchManuscript,
  fetchManuscripts,
  replaceManuscriptCitations,
  reviewManuscript,
  searchLiterature,
  setManuscriptAnnotationStatus,
  signoffManuscript,
  stageManuscriptDeposit,
} from '@/lib/api';
export type { ManuscriptDetail, ManuscriptSummary } from '@/lib/api';
