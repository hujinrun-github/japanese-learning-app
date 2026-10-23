// Vitest runs this regression check in Node; the browser build intentionally omits Node typings.
// @ts-expect-error -- node:fs is only used by this test file.
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const variablesCss = readFileSync(new URL('./variables.css', import.meta.url), 'utf8')
const globalCss = readFileSync(new URL('./global.css', import.meta.url), 'utf8')

describe('learner theme background', () => {
  const checks = [
    {
      name: 'uses a consistent light color scheme',
      css: variablesCss,
      expected: /:root\s*{[^}]*color-scheme:\s*light;/s,
    },
    {
      name: 'does not partially switch the app from light to dark',
      css: variablesCss,
      rejected: /@media\s*\(prefers-color-scheme:\s*dark\)/,
    },
    {
      name: 'uses the shared page background without decorative gradients',
      css: globalCss,
      expected: /body\s*{[^}]*background:\s*var\(--color-bg\);/s,
      rejected: /background-attachment:\s*fixed;/,
    },
  ]

  for (const check of checks) {
    it(check.name, () => {
      if (check.expected) {
        expect(check.css).toMatch(check.expected)
      }
      if (check.rejected) {
        expect(check.css).not.toMatch(check.rejected)
      }
    })
  }
})
