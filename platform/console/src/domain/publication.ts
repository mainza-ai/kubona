// Publication domain boundary: manuscript lifecycle transport used by the
// Results Studio publication surface.
export {
  buildManuscript,
  createManuscript,
  createManuscriptAnnotation,
  deleteManuscript,
  downloadManuscriptArtifact,
  exportManuscriptBundle,
  fetchFormalVerifications,
  fetchManuscript,
  fetchManuscripts,
  replaceManuscriptCitations,
  reviewManuscript,
  searchLiterature,
  setManuscriptAnnotationStatus,
  signoffManuscript,
  stageManuscriptDeposit,
  submitFormalVerification,
  updateManuscriptDraft,
  verifyManuscript,
} from '@/lib/api';
export type {
  FormalVerificationEntry,
  ManuscriptDetail,
  ManuscriptExportBundle,
  ManuscriptSummary,
  ManuscriptVerificationReport,
} from '@/lib/api';
