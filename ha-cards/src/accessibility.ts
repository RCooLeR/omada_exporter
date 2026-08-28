export type InspectableKind = "device" | "client";

/** Build the concise action name exposed by dashboard selection controls. */
export function inspectionAriaLabel(kind: InspectableKind, name: string): string {
  const normalizedName = name.trim() || `unnamed ${kind}`;
  return `Inspect ${kind} ${normalizedName}`;
}
