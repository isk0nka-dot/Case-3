import fs from 'fs'
import path from 'path'
import sharp from 'sharp'
import pngToIco from 'png-to-ico'
import { fileURLToPath } from 'url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

const PUBLIC_DIR = path.resolve(__dirname, '../public')
const INPUT_SVG = path.join(PUBLIC_DIR, 'favicon.svg')

const SIZES = {
  'favicon-32x32-v2.png': 32,
  'favicon-16x16-v2.png': 16,
  'apple-touch-icon-v2.png': 180,
  'favicon-192x192-v2.png': 192,
  'favicon-512x512-v2.png': 512
}

async function generateFavicons() {
  console.log('Generating favicons...')

  if (!fs.existsSync(INPUT_SVG)) {
    console.error('favicon.svg not found in public directory!')
    process.exit(1)
  }

  // Generate PNGs
  for (const [filename, size] of Object.entries(SIZES)) {
    const outputPath = path.join(PUBLIC_DIR, filename)
    await sharp(INPUT_SVG)
      .resize(size, size)
      .png()
      .toFile(outputPath)
    console.log(`Generated ${filename}`)
  }

  // Generate ICO (from 32x32 and 16x16)
  const icoPath = path.join(PUBLIC_DIR, 'favicon-v2.ico')
  const buf = await pngToIco([
    path.join(PUBLIC_DIR, 'favicon-16x16-v2.png'),
    path.join(PUBLIC_DIR, 'favicon-32x32-v2.png')
  ])
  fs.writeFileSync(icoPath, buf)
  console.log('Generated favicon-v2.ico')

  // Generate Webmanifest
  const manifest = {
    name: 'Argus AI',
    short_name: 'Argus AI',
    icons: [
      {
        src: '/favicon-192x192-v2.png',
        sizes: '192x192',
        type: 'image/png'
      },
      {
        src: '/favicon-512x512-v2.png',
        sizes: '512x512',
        type: 'image/png'
      }
    ],
    theme_color: '#121820',
    background_color: '#121820',
    display: 'standalone'
  }

  fs.writeFileSync(
    path.join(PUBLIC_DIR, 'site.webmanifest'),
    JSON.stringify(manifest, null, 2)
  )
  console.log('Generated site.webmanifest')

  // Optionally remove the old favicon.ico if you want, but we will leave it for safety
}

generateFavicons().catch((err) => {
  console.error(err)
  process.exit(1)
})
