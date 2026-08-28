import type { EChartsOption } from "echarts";
import { init, type EChartsType } from "echarts/core";
import type { ReactiveController, ReactiveControllerHost } from "lit";

interface ResizeObserverHandle {
  observe(target: Element): void;
  disconnect(): void;
}

export interface ChartControllerPlatform {
  initialize(element: HTMLElement): EChartsType;
  createResizeObserver(callback: () => void): ResizeObserverHandle;
  requestFrame(callback: FrameRequestCallback): number;
  cancelFrame(handle: number): void;
}

const browserPlatform: ChartControllerPlatform = {
  initialize: (element) => init(element, undefined, { renderer: "canvas", useDirtyRect: true }),
  createResizeObserver: (callback) => new ResizeObserver(callback),
  requestFrame: (callback) => requestAnimationFrame(callback),
  cancelFrame: (handle) => cancelAnimationFrame(handle)
};

export function deviceResourceSummarySignature(
  deviceName: string,
  cpuPercent: number,
  memoryPercent: number,
  clientCount: number
): string {
  return ["resources", deviceName, cpuPercent, memoryPercent, clientCount].join("|");
}

/** Owns ECharts instances and browser observers across the full Lit connection lifecycle. */
export class ChartController implements ReactiveController {
  private readonly host: ReactiveControllerHost & HTMLElement;
  private readonly syncCharts: () => void;
  private readonly platform: ChartControllerPlatform;
  private readonly charts = new Map<string, EChartsType>();
  private readonly elements = new Map<string, HTMLElement>();
  private readonly signatures = new Map<string, string>();
  private observer: ResizeObserverHandle | undefined;
  private resizeFrame: number | undefined;
  private connected = false;
  private wasDisconnected = false;

  public constructor(
    host: ReactiveControllerHost & HTMLElement,
    syncCharts: () => void,
    platform: ChartControllerPlatform = browserPlatform
  ) {
    this.host = host;
    this.syncCharts = syncCharts;
    this.platform = platform;
    host.addController(this);
  }

  public hostConnected(): void {
    this.connected = true;
    this.observer ??= this.platform.createResizeObserver(() => this.scheduleResize());
    this.observer.observe(this.host);
    if (this.wasDisconnected) {
      this.wasDisconnected = false;
      this.host.requestUpdate();
    }
  }

  public hostUpdated(): void {
    if (this.connected) {
      this.syncCharts();
    }
  }

  public hostDisconnected(): void {
    this.connected = false;
    this.wasDisconnected = true;
    this.observer?.disconnect();
    if (this.resizeFrame !== undefined) {
      this.platform.cancelFrame(this.resizeFrame);
      this.resizeFrame = undefined;
    }
    this.clear();
  }

  public render(key: string, element: HTMLElement, option: EChartsOption, signature: string): void {
    const currentElement = this.elements.get(key);
    let chart = this.charts.get(key);
    if (chart && currentElement !== element) {
      chart.dispose();
      this.charts.delete(key);
      this.elements.delete(key);
      this.signatures.delete(key);
      chart = undefined;
    }

    if (!chart) {
      chart = this.platform.initialize(element);
      this.charts.set(key, chart);
      this.elements.set(key, element);
      chart.resize();
    }

    if (this.signatures.get(key) === signature) {
      return;
    }

    chart.setOption(option, { notMerge: true, lazyUpdate: true, silent: true });
    this.signatures.set(key, signature);
  }

  public clear(): void {
    this.charts.forEach((chart) => chart.dispose());
    this.charts.clear();
    this.elements.clear();
    this.signatures.clear();
  }

  private scheduleResize(): void {
    if (this.resizeFrame !== undefined) {
      return;
    }

    this.resizeFrame = this.platform.requestFrame(() => {
      this.resizeFrame = undefined;
      this.charts.forEach((chart) => chart.resize());
    });
  }
}
