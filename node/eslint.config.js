import js from "@eslint/js";
import tseslint from "typescript-eslint";

export default tseslint.config(
  {
    ignores: ["dist/**", "coverage/**", "node_modules/**"],
  },
  js.configs.recommended,
  ...tseslint.configs.recommendedTypeChecked,
  {
    languageOptions: {
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
  },
  {
    // The eslint flat config file itself is not part of the TS project.
    files: ["eslint.config.js", "vitest.config.ts", "tsup.config.ts"],
    ...tseslint.configs.disableTypeChecked,
  },
);
