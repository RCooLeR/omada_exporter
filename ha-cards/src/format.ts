export function toNumber(value: unknown): number {
  if (typeof value === "number") {
    return Number.isFinite(value) ? value : 0;
  }

  if (typeof value === "boolean") {
    return value ? 1 : 0;
  }

  if (typeof value === "string") {
    if (value === "on") {
      return 1;
    }

    if (value === "off") {
      return 0;
    }

    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : 0;
  }

  return 0;
}

export function formatSpeedMbps(mbps: number): string {
  if (!mbps) {
    return "-";
  }

  if (mbps >= 1000) {
    return `${(mbps / 1000).toFixed(1)} Gbps`;
  }

  return `${mbps.toFixed(0)} Mbps`;
}

export function formatRateBytes(value: number): string {
  if (!value) {
    return "-";
  }

  const units = ["B/s", "KB/s", "MB/s", "GB/s", "TB/s"];
  let current = value;
  let index = 0;

  while (current >= 1024 && index < units.length - 1) {
    current /= 1024;
    index += 1;
  }

  return `${current.toFixed(current >= 10 ? 0 : 1)} ${units[index]}`;
}

export function formatBytes(value: number): string {
  if (!value) {
    return "-";
  }

  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let current = value;
  let index = 0;

  while (current >= 1024 && index < units.length - 1) {
    current /= 1024;
    index += 1;
  }

  return `${current.toFixed(current >= 10 ? 0 : 1)} ${units[index]}`;
}

export function formatPercent(value: number): string {
  if (!Number.isFinite(value)) {
    return "-";
  }

  return `${value.toFixed(0)}%`;
}

export function formatUptimeMinutes(value: number): string {
  if (!value) {
    return "-";
  }

  const totalMinutes = Math.floor(value);
  const days = Math.floor(totalMinutes / 1440);
  const hours = Math.floor((totalMinutes % 1440) / 60);
  const minutes = totalMinutes % 60;

  if (days > 0) {
    return `${days}d ${hours}h`;
  }

  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }

  return `${minutes}m`;
}

export function formatUptimeSeconds(value: number): string {
  if (!value) {
    return "-";
  }

  const totalSeconds = Math.floor(value);
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);

  if (days > 0) {
    return `${days}d ${hours}h`;
  }

  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }

  return `${minutes}m`;
}

export function formatLatency(value: number): string {
  if (!value) {
    return "-";
  }

  return `${value.toFixed(0)} ms`;
}

export function qualityLabel(signalPct: number, rssi: number): string {
  if (signalPct >= 85 || rssi >= -55) {
    return "Excellent";
  }

  if (signalPct >= 70 || rssi >= -65) {
    return "Good";
  }

  if (signalPct >= 50 || rssi >= -75) {
    return "Fair";
  }

  return "Weak";
}
