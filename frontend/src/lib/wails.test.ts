import { describe, expect, it } from "vitest";
import { parseWailsError, translateWailsError } from "./wails";
import "@/i18n";

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
