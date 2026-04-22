export interface HomeAssistant {
  states: Record<string, HassEntity>;
  themes?: {
    darkMode?: boolean;
  };
}

export interface LovelaceCardConfig {
  type: string;
  title?: string;
  site?: string;
  logo_mode?: "light" | "dark" | "auto";
  device_limit?: number;
  client_limit?: number;
}

export interface HassEntity {
  entity_id: string;
  state: string;
  attributes: Record<string, unknown>;
  last_updated?: string;
}

export interface MetricRecord {
  [metricName: string]: number;
}

export interface DeviceRecord {
  key: string;
  name: string;
  type: string;
  model: string;
  status: string;
  ip: string;
  mac: string;
  version: string;
  site: string;
  attrs: Record<string, unknown>;
  metrics: MetricRecord;
  ports: PortRecord[];
  radios: RadioRecord[];
  clients: ClientRecord[];
}

export interface PortRecord {
  key: string;
  deviceMac: string;
  name: string;
  port: string;
  kind: string;
  operation: string;
  status: string;
  poe: boolean;
  attrs: Record<string, unknown>;
  metrics: MetricRecord;
  clients: ClientRecord[];
}

export interface RadioRecord {
  key: string;
  deviceMac: string;
  band: string;
  attrs: Record<string, unknown>;
  metrics: MetricRecord;
}

export interface ClientRecord {
  key: string;
  name: string;
  mac: string;
  ip: string;
  vendor: string;
  hostName: string;
  category: string;
  clientType: string;
  wireless: boolean;
  apMac: string;
  apName: string;
  switchMac: string;
  switchName: string;
  gatewayMac: string;
  gatewayName: string;
  port: string;
  ssid: string;
  vlanId: string;
  wifiMode: string;
  site: string;
  attrs: Record<string, unknown>;
  metrics: MetricRecord;
}

export interface LinkRow {
  key: string;
  name: string;
  status: string;
  attrs: Record<string, unknown>;
  metrics: MetricRecord;
}

export interface SiteSummary {
  site: string;
  wiredClients: number;
  wirelessClients: number;
  devicesOnline: number;
  devicesOffline: number;
  gateways: number;
  switches: number;
  aps: number;
  controllers: number;
  maxCpu: number;
  maxCpuDevice: string;
  maxMem: number;
  maxMemDevice: string;
}

export interface DashboardModel {
  siteSummary: SiteSummary;
  devices: DeviceRecord[];
  clients: ClientRecord[];
  isps: LinkRow[];
  vpns: LinkRow[];
  wans: LinkRow[];
}
