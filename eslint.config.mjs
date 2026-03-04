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
      'argus/no-duplicate-utils': 'warn',
      '@typescript-eslint/no-unused-vars': 'warn',
      '@typescript-eslint/no-explicit-any': 'warn',
      '@stylistic/max-statements-per-line': 'warn',
      'vue/no-mutating-props': 'warn',
      'vue/return-in-computed-property': 'warn',
      '@typescript-eslint/unified-signatures': 'warn',
      '@typescript-eslint/no-duplicate-enum-values': 'warn',
      'no-useless-escape': 'warn'
    }
  }
)
