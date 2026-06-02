import resolve from '@rollup/plugin-node-resolve';
import typescript from '@rollup/plugin-typescript';
import terser from '@rollup/plugin-terser';

export default {
  input: 'src/index.ts',
  output: [
    {
      file: 'dist/argus-sdk.esm.js',
      format: 'es',
      sourcemap: true,
    },
    {
      file: 'dist/argus-sdk.umd.js',
      format: 'umd',
      name: 'ArgusSDK',
      sourcemap: true,
      globals: {
        '@mediapipe/tasks-vision': 'MediaPipeVision',
      },
    },
  ],
  external: ['@mediapipe/tasks-vision'],
  plugins: [
    resolve({ browser: true }),
    typescript({
      tsconfig: './tsconfig.build.json',
      declaration: true,
      declarationDir: 'dist',
    }),
    terser({
      compress: { passes: 2 },
      format: { comments: false },
    }),
  ],
};
