export const favoriteKinds = [
  'word', 'grammar', 'lesson', 'listening', 'listening_sentence',
  'speaking', 'shadowing', 'writing', 'translation', 'note',
] as const

export type FavoriteKind = typeof favoriteKinds[number]

export interface FavoriteTarget {
  kind: FavoriteKind
  key: string
}

export interface FavoriteItem extends FavoriteTarget {
  title: string
  detail: string
  jlpt_level: string
  path: string
}

export function favoriteKey({ kind, key }: FavoriteTarget) {
  return `${kind}/${key}`
}
