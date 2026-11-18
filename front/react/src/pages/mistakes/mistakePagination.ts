export function mistakePageWithinTotal(page: number, total: number, size: number): number {
  return Math.max(1, Math.min(page, Math.ceil(total / size)))
}
