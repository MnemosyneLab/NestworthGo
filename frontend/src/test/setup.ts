import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";
import i18n from "@/i18n";

afterEach(() => {
  cleanup();
  // i18next is a module-level singleton; a test that changes the active
  // language (e.g. saving Settings) must not leak that into the next
  // test in the same file.
  void i18n.changeLanguage(i18n.options.lng);
});
