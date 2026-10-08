import next from "eslint-config-next";

const config = [
  ...next,
  {
    ignores: [".next/**", "node_modules/**", "public/**", "playwright-report/**", "test-results/**", "next-env.d.ts"],
  },
  {
    rules: {
      "react/no-unescaped-entities": "off",
    },
  },
];

export default config;
