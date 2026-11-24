import { defineConfig, type Connect, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'
import { createRequire } from 'node:module'
import { createReadStream } from 'node:fs'

const require = createRequire(import.meta.url)

function rawKuromojiDictionary(): Plugin {
  function serve(middlewares: Connect.Server, directory: string) {
    middlewares.use('/kuromoji-dict', (req, res, next) => {
      const file = req.url?.split('?')[0].match(/^\/([a-z_]+\.dat\.gz)$/)?.[1]
      if (!file || (req.method !== 'GET' && req.method !== 'HEAD')) return next()
      const stream = createReadStream(path.resolve(directory, 'kuromoji-dict', file))
      stream.once('open', () => {
        // The dictionary loader decompresses these bytes itself.
        res.setHeader('Content-Type', 'application/gzip')
        if (req.method === 'HEAD') {
          stream.destroy()
          res.end()
        } else {
          stream.pipe(res)
        }
      })
      stream.once('error', (error: NodeJS.ErrnoException) => {
        if (res.headersSent) res.destroy(error)
        else if (error.code === 'ENOENT') {
          res.statusCode = 404
          res.end()
        } else {
          next(error)
        }
      })
      res.once('close', () => stream.destroy())
    })
  }

  return {
    name: 'raw-kuromoji-dictionary',
    configureServer({ middlewares, config }) {
      serve(middlewares, config.publicDir)
    },
    configurePreviewServer({ middlewares, config }) {
      serve(middlewares, path.resolve(config.root, config.build.outDir))
    },
  }
}

// https://vite.dev/config/
export default defineConfig(({ mode, isPreview }) => ({
  plugins: [react(), rawKuromojiDictionary()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      // Use the packaged browser build, which includes its dictionary loader's path helpers.
      ...(mode !== 'test' && !isPreview && { kuromoji: require.resolve('kuromoji/build/kuromoji.js') }),
    },
  },
  server: {
    port: 35173,
    proxy: {
      '/api': {
        target: 'http://localhost:30081',
        changeOrigin: true,
      },
      '/audio': {
        target: 'http://localhost:30081',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: '../../front/dist',
    emptyOutDir: true,
  },
}))
