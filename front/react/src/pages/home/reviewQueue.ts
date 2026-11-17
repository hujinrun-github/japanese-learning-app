// Home dashboard review queue. Kept as plain data so the mapping between a
// label, its counter and its destination can be regression-tested.
export const REVIEW_ITEMS = [
  { key: 'word', icon: 'A', to: '/words/review', labelKey: 'home.reviewQueue.word', detailKey: 'home.reviewQueue.wordDetail', countFrom: 'module' },
  { key: 'grammar', icon: '文', to: '/grammar', labelKey: 'home.reviewQueue.grammar', detailKey: 'home.reviewQueue.grammarDetail', countFrom: 'module' },
  // The mistake book has its own page; its badge comes from the real mistake count.
  { key: 'writing', icon: '📕', to: '/mistakes', labelKey: 'home.reviewQueue.writing', detailKey: 'home.reviewQueue.writingDetail', countFrom: 'mistakes' },
  { key: 'speaking', icon: '🎧', to: '/speaking', labelKey: 'home.reviewQueue.speaking', detailKey: 'home.reviewQueue.speakingDetail', countFrom: 'module' },
] as const
