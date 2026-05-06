declare module 'kuromojin' {
  interface Token {
    surface_form: string
    reading?: string
    pos: string
    basic_form: string
  }

  export function tokenize(text: string): Promise<Token[]>
}
