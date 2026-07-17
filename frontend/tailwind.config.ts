import type { Config } from "tailwindcss";
import forms from "@tailwindcss/forms";
import typography from "@tailwindcss/typography";

export default {
  content: [
    "../pkg/weave/templates/**/*.gohtml",
    // Inline gohtml templates embedded in Go source (e.g. pkg/weave/ontology/pages_browse.go).
    // Without this, Tailwind JIT misses classes like `bg-sky-700` referenced only inside string
    // literals, and buttons styled with those classes render as white-on-white (invisible).
    "../pkg/weave/**/*.go",
    "./src/**/*.{svelte,ts,js}",
  ],
  theme: {
    extend: {
      colors: {
        "pletka-primary": "#92ccc8",
        "pletka-secondary": "#f58031",
      },
    },
  },
  plugins: [forms, typography],
} satisfies Config;
