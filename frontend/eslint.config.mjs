import nextVitals from "eslint-config-next/core-web-vitals";
import tseslint from "typescript-eslint";

export default tseslint.config(
  {
    ignores: [
      ".next/**",
      "node_modules/**",
      "lib/errors/codes.gen.ts",
      "playwright-report/**",
      "test-results/**",
      "next-env.d.ts",
    ],
  },
  ...nextVitals,
  ...tseslint.configs.strict,
  {
    // eslint-plugin-react's "detect" calls context.getFilename, which ESLint 10 removed; pin the version instead.
    settings: { react: { version: "19.3" } },
    rules: {
      "@typescript-eslint/no-magic-numbers": [
        "error",
        {
          ignore: [0, 1, -1],
          ignoreArrayIndexes: true,
          ignoreEnums: true,
          ignoreReadonlyClassProperties: true,
          ignoreTypeIndexes: true,
        },
      ],
      "@typescript-eslint/consistent-type-imports": ["error", { fixStyle: "inline-type-imports" }],
      "max-lines-per-function": ["error", { max: 50, skipBlankLines: true, skipComments: true }],
      complexity: ["error", 10],
      "max-depth": ["error", 3],
    },
  },
  {
    // JSX markup inflates line counts; components get a looser limit, logic in .ts files keeps 50.
    files: ["**/*.tsx"],
    rules: { "max-lines-per-function": ["error", { max: 120, skipBlankLines: true, skipComments: true }] },
  },
  {
    // Fixtures are data tables (scores, page numbers), like test tables.
    files: ["**/*.test.ts", "**/*.test.tsx", "e2e/**", "*.config.*", "mock-core/fixtures/**"],
    rules: { "@typescript-eslint/no-magic-numbers": "off", "max-lines-per-function": "off" },
  },
);
