import { describe, expect, it, vi } from "vitest";
import { parseWailsError, translateWailsError } from "./wails";
import i18n from "@/i18n";

// Pure wire-error helpers must not initialize desktop DOM handlers/timers.
vi.mock("@wailsio/runtime", () => { throw new Error("wire helpers loaded the desktop SDK"); });

describe("parseWailsError", () => {
  it("parses a valid WireError JSON message", () => {
    const error = new Error(JSON.stringify({ code: "validation", field: "name", message: "must not be empty" }));
    const parsed = parseWailsError(error);
    expect(parsed).toEqual({ code: "validation", field: "name", message: "must not be empty" });
  });

  it("falls back to a generic internal error for malformed JSON", () => {
    const error = new Error("{not valid json");
    const parsed = parseWailsError(error);
    expect(parsed.code).toBe("internal");
  });

  it("falls back to a generic internal error for a plain non-JSON message", () => {
    const error = new Error("connection refused");
    const parsed = parseWailsError(error);
    expect(parsed.code).toBe("internal");
    expect(parsed.message.length).toBeGreaterThan(0);
  });

  it("does not crash on a non-Error rejection value", () => {
    const parsed = parseWailsError(undefined);
    expect(parsed.code).toBe("internal");
  });

  it("rejects a JSON object missing the required code field", () => {
    const error = new Error(JSON.stringify({ message: "no code here" }));
    const parsed = parseWailsError(error);
    expect(parsed.code).toBe("internal");
  });
});

describe("translateWailsError", () => {
  it("translates by stable code, not by message text", () => {
    const message = translateWailsError({ code: "not_found", message: "some backend message that should be ignored" });
    expect(message).not.toContain("some backend message");
    expect(message.length).toBeGreaterThan(0);
  });

  it("falls back to the internal error code for an unrecognized code", () => {
    const message = translateWailsError({ code: "some_future_code_not_yet_localized", message: "x" });
    expect(message.length).toBeGreaterThan(0);
  });

  it("prefixes a localized field label when the field is known", () => {
    const message = translateWailsError({ code: "validation", field: "accountId", message: "x" });
    expect(message).toContain(":");
  });
});

it.each([["startOn", "Start date"], ["maturityOn", "Maturity date"], ["earlyFee", "Early fee"]])("labels product validation field %s", (field, label) => {
  expect(translateWailsError({ code: "validation", field, message: "invalid" })).toContain(label);
});

it.each(["en", "zh-CN", "zh-TW"])("translates mobile validation by field and preserves other error codes in %s", async (language) => {
  await i18n.changeLanguage(language);
  for (const field of ["listen", "token"]) {
    expect(translateWailsError({ code: "validation", field, message: "untrusted backend detail" })).toBe(`${i18n.t(`mobile.${field}`)}: ${i18n.t(`mobile.${field}Validation`)}`);
  }
  expect(translateWailsError({ code: "internal", field: "listen", message: "x" })).toContain(i18n.t("internal", { ns: "errorCode" }));
  await i18n.changeLanguage("en");
});

it("recovers a retained callService payload without parsing its old translated message", () => {
  const wireError = { code: "validation", field: "token", message: "untrusted detail" };
  const retained = Object.assign(new Error("previous language"), { wireError });
  expect(parseWailsError(retained)).toEqual(wireError);
});

it.each([
  ["en", "Use 120 characters or fewer.", "Select at least one owner before continuing or saving."],
  ["zh-CN", "请使用不超过 120 个字符。", "继续或保存前，请至少选择一名所有权人。"],
  ["zh-TW", "請使用不超過 120 個字元。", "繼續或儲存前，請至少選擇一名所有權人。"],
  ["ja", "120 文字以内で入力してください。", "続行または保存する前に、所有者を 1 人以上選択してください。"],
])("localizes length constraints and actionable owner guidance in %s", async (language, expected, ownerHint) => {
  try {
    await i18n.changeLanguage(language);
    for (const field of ["name", "label"]) {
      const wire = parseWailsError(new Error(JSON.stringify({ code: "validation", field, maxLength: 120, message: "private backend detail" })));
      expect(translateWailsError(wire)).toContain(expected);
      expect(translateWailsError(wire)).not.toContain("private backend detail");
    }
    expect(i18n.t("accounts.ownershipHint")).toBe(ownerHint);
    expect(translateWailsError({ code: "validation", field: "note", maxLength: 2000, message: "ignored" })).toContain("2000");
  } finally { await i18n.changeLanguage("en"); }
});

it.each([undefined, 0, -1, 1.5, NaN, Infinity, "120", {}])("retains generic feedback for invalid length metadata %s", (maxLength) => {
  const wire = parseWailsError(new Error(JSON.stringify({ code: "validation", field: "name", maxLength, message: "must be 120 characters or fewer" })));
  expect(translateWailsError(wire)).toBe(translateWailsError({ code: "validation", field: "name", message: "ignored" }));
});

it("does not interpret length metadata on other error codes", () => {
  expect(translateWailsError({ code: "internal", maxLength: 120, message: "ignored" })).toBe(translateWailsError({ code: "internal", message: "ignored" }));
});
