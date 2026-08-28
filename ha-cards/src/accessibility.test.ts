import { describe, expect, it } from "vitest";
import { inspectionAriaLabel } from "./accessibility";

describe("inspectionAriaLabel", () => {
  it("names device and client selection actions", () => {
    expect(inspectionAriaLabel("device", "Core Switch")).toBe("Inspect device Core Switch");
    expect(inspectionAriaLabel("client", " Workstation ")).toBe("Inspect client Workstation");
  });

  it("provides a useful fallback for an unnamed record", () => {
    expect(inspectionAriaLabel("client", "   ")).toBe("Inspect client unnamed client");
  });
});
