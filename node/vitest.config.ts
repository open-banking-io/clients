import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    coverage: {
      provider: "v8",
      reportsDirectory: "coverage",
      reporter: ["text", "lcov", "cobertura"],
      include: ["src/**/*.ts"],
      // index.ts is re-exports only; models.ts is pure type declarations (no runtime code).
      exclude: ["src/index.ts", "src/models.ts"],
    },
  },
});
