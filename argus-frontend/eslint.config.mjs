// @ts-check
import withNuxt from './.nuxt/eslint.config.mjs'
import noDuplicateUtils from './eslint-rules/no-duplicate-utils.js'

// withNuxt(config) places the custom config BEFORE Nuxt defaults,
// so Nuxt's error-level rules override them. Use .append() to place
// custom rules AFTER Nuxt defaults, ensuring they take effect.
export default withNuxt().append(
  // -------------------------------------------------------------------------
  // Argus AI — Custom ESLint Rules
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
      '@typescript-eslint/no-unused-vars': 'off',
      '@typescript-eslint/no-explicit-any': 'off',
      '@stylistic/max-statements-per-line': 'off',
      'vue/no-mutating-props': 'warn',
      'vue/return-in-computed-property': 'off',
      '@typescript-eslint/unified-signatures': 'off',
      '@typescript-eslint/no-duplicate-enum-values': 'off',
      'no-useless-escape': 'off'
    }
  }
)
