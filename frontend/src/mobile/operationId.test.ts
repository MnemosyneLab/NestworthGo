import { expect, it, vi } from "vitest";
import { mobileOperationId } from "./operationId";

it("generates UUID v4 operation identities without secure-context randomUUID", () => {
  const randomUUID = vi.spyOn(crypto, "randomUUID").mockImplementation(() => {
    throw new Error("insecure context");
  });
  try {
    const ids = Array.from({ length: 10 }, mobileOperationId);
    for (const id of ids)
      expect(id).toMatch(
        /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
      );
    expect(new Set(ids).size).toBe(ids.length);
    expect(randomUUID).not.toHaveBeenCalled();
  } finally {
    randomUUID.mockRestore();
  }
});
