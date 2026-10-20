import js from "@eslint/js";
import globals from "globals";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["dist", "bindings", "node_modules"] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
    },
    plugins: {
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "react-refresh/only-export-components": "off",
      "@typescript-eslint/no-unused-vars": ["warn", { argsIgnorePattern: "^_" }],
    },
  },
  {
    files: ["src/**/*.{ts,tsx}"],
    ignores: ["src/**/*.test.*", "src/test/**", "src/components/ui/**"],
    rules: {
      "no-restricted-imports": ["error", { patterns: [
        { group: ["react-day-picker", "@base-ui/react", "@base-ui/react/*"], message: "Reuse components/ui wrappers; extend the canonical control there first." },
        { group: ["**/ui/calendar"], message: "Date entry uses DatePicker. Calendar is its internal primitive; analytics visualizations are separate." },
      ] }],
      "no-restricted-syntax": ["error",
        { selector: 'JSXAttribute[name.name="type"][value.value=/^(date|datetime-local|time|month|week)$/]', message: "Use DatePicker/TimePicker with civil-date bounds, not native temporal inputs." },
        { selector: 'JSXAttribute[name.name="type"] > JSXExpressionContainer > Literal[value=/^(date|datetime-local|time|month|week)$/]', message: "Use DatePicker/TimePicker with civil-date bounds, not native temporal inputs." },
        { selector: 'JSXOpeningElement[name.name=/^(select|dialog)$/]', message: "Use NativeSelect, Sheet or AlertDialog from components/ui." },
        { selector: 'JSXAttribute[name.name="role"][value.value=/^(dialog|alertdialog)$/]', message: "Use Sheet or AlertDialog to retain focus, Escape and dismissal behavior." },
        { selector: 'JSXAttribute[name.name="role"] > JSXExpressionContainer > Literal[value=/^(dialog|alertdialog)$/]', message: "Use Sheet or AlertDialog to retain focus, Escape and dismissal behavior." },
        { selector: "JSXText[value=/\\d{4}-\\d{2}-\\d{2}/]", message: "Format civil dates for display. Do not render a raw ISO date in JSX text." },
        { selector: "JSXElement > JSXExpressionContainer > Literal[value=/^\\d{4}-\\d{2}-\\d{2}(?:T[\\d:.Z+-]*)?$/]", message: "Format civil dates for display. Do not render a raw ISO date in JSX text." },
        { selector: "JSXFragment > JSXExpressionContainer > Literal[value=/^\\d{4}-\\d{2}-\\d{2}(?:T[\\d:.Z+-]*)?$/]", message: "Format civil dates for display. Do not render a raw ISO date in JSX text." },
        { selector: "JSXText[value=/^\\s*[a-z][a-z0-9]*(?:_[a-z0-9]+)+\\s*$/]", message: "Map reason codes through locale text. Do not render a raw reason code in JSX." },
        { selector: "JSXElement > JSXExpressionContainer > Literal[value=/^[a-z][a-z0-9]*(?:_[a-z0-9]+)+$/]", message: "Map reason codes through locale text. Do not render a raw reason code in JSX." },
        { selector: "JSXFragment > JSXExpressionContainer > Literal[value=/^[a-z][a-z0-9]*(?:_[a-z0-9]+)+$/]", message: "Map reason codes through locale text. Do not render a raw reason code in JSX." },
      ],
    },
  },
);
