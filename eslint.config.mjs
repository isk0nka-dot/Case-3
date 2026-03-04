// @ts-check
import withNuxt from './.nuxt/eslint.config.mjs'
import noDuplicateUtils from './eslint-rules/no-duplicate-utils.js'

export default withNuxt(
  // -------------------------------------------------------------------------
  // Argus AI — Custom ESLint Rules
  // -------------------------------------------------------------------------
  // Enforces the "Utility Genocide" policy: local re-declarations of
  // centralized composable utilities are build-blocking errors.
  // -------------------------------------------------------------------------
  {
    plugins: {
      argus: {
        rules: {
          'no-duplicate-utils': noDuplicateUtils
        }
      }
    },
    rules: {
      'argus/no-duplicate-utils': 'error'
    }
  }
)
