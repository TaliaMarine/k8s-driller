import pluginVue from 'eslint-plugin-vue'
import vueTsEslintConfig from '@vue/eslint-config-typescript'
import vueEslintConfigPrettier from '@vue/eslint-config-prettier'

export default [
  { ignores: ['dist/**'] },
  ...pluginVue.configs['flat/recommended'],
  ...vueTsEslintConfig(),
  vueEslintConfigPrettier, // disables stylistic rules that fight Prettier's own formatting
  {
    rules: {
      'vue/multi-word-component-names': 'off',
    },
  },
]
