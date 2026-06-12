import { describe, expect, it } from "vitest";
import {
  formatBytes,
  formatRateBytes,
  formatSpeedKbps,
  formatSpeedMbps,
  formatUptimeSeconds,
  qualityLabel,
  toNumber
} from "./format";

describe("format helpers", () => {
  it("normalizes Home Assistant states into numbers", () => {
    expect(toNumber(7)).toBe(7);
    expect(toNumber(true)).toBe(1);
    expect(toNumber(false)).toBe(0);
    expect(toNumber("on")).toBe(1);
    expect(toNumber("off")).toBe(0);
    expect(toNumber("42.5")).toBe(42.5);
    expect(toNumber("unknown")).toBe(0);
  });

  it("formats common network values", () => {
    expect(formatSpeedMbps(1500)).toBe("1.5 Gbps");
    expect(formatSpeedKbps(866700)).toBe("867 Mbps");
    expect(formatRateBytes(1536)).toBe("1.5 KB/s");
    expect(formatBytes(10 * 1024 * 1024)).toBe("10 MB");
    expect(formatUptimeSeconds(90061)).toBe("1d 1h");
  });

  it("labels wireless quality from either signal percentage or RSSI", () => {
    expect(qualityLabel(90, -80)).toBe("Excellent");
    expect(qualityLabel(0, -60)).toBe("Good");
    expect(qualityLabel(55, -90)).toBe("Fair");
    expect(qualityLabel(10, -90)).toBe("Weak");
  });
});
