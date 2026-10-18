// Resolve WAN display status without treating missing telemetry as an outage.
import type { LinkRow } from "./ha-types";

/** Parse a reported link metric, preserving unavailable and missing readings. */
export function linkMetricValue(state: string): number | undefined {
  const value = state.trim();
  if (value === "on") return 1;
  if (value === "off") return 0;
  if (!value) return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

/** Prefer the matched WAN's connection state over the legacy ISP status enum. */
export function wanDisplayStatus(isp: LinkRow, wan?: LinkRow): {
  label: "Online" | "Offline" | "Connecting" | "Disconnecting" | "Unknown";
  className: "status-up" | "status-down" | "status-unknown";
} {
  if (wan?.metrics.omada_wan_status === 0) {
    return { label: "Offline", className: "status-down" };
  }
  // Omada's WAN UI distinguishes connection transitions from connected state.
  switch (wan?.metrics.omada_wan_internet_state) {
    case 0: return { label: "Offline", className: "status-down" };
    case 1: return { label: "Online", className: "status-up" };
    case 2: return { label: "Connecting", className: "status-unknown" };
    case 3: return { label: "Disconnecting", className: "status-unknown" };
  }
  // Other ISP codes (including Fusion's 2) are not binary connectivity states.
  switch (isp.metrics.omada_isp_status) {
    case 0: return { label: "Offline", className: "status-down" };
    case 1: return { label: "Online", className: "status-up" };
    default: return { label: "Unknown", className: "status-unknown" };
  }
}
