/**
 * Ambient types for `.json5` fixture imports.
 *
 * `vitest.config.ts` has a `vite-plugin-json5` that turns these into modules at
 * run time, so the imports have always WORKED — they were just invisible to
 * `tsc`, because `tsconfig.json` excluded the whole `tests/` tree. Bringing that
 * tree under the typechecker (TRA-1253) is what surfaced them.
 *
 * Deliberately loose. These are recorded CS108 captures, not a schema this repo
 * controls, and the consumers in `tests/data/` already narrow what they take out
 * of them (`payloads.map((p: number[]) => ...)`). Inventing a precise interface
 * here would assert a shape no fixture is checked against.
 */
declare module '*.json5' {
  const value: {
    payloads: number[][];
    [key: string]: unknown;
  };
  export default value;
}
