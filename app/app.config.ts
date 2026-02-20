export default defineAppConfig({
  ui: {
    colors: {
      primary: 'sky',
      neutral: 'slate'
    },
    // Override toaster default position to top-right globally.
    // This ensures all Toast notifications appear in the top-right corner
    // with a slide-in-from-top animation (built into Nuxt UI).
    toaster: {
      defaultVariants: {
        position: 'top-right' as const
      }
    }
  }
})
