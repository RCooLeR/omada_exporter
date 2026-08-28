import type { EChartsType } from "echarts/core";
import type { ReactiveController, ReactiveControllerHost } from "lit";
import { describe, expect, it, vi } from "vitest";
import { ChartController, deviceResourceSummarySignature, type ChartControllerPlatform } from "./chart-controller";

function setupController() {
  const controllers: ReactiveController[] = [];
  const host = {
    addController: (controller: ReactiveController) => controllers.push(controller),
    removeController: vi.fn(),
    requestUpdate: vi.fn(),
    updateComplete: Promise.resolve(true)
  } as unknown as ReactiveControllerHost & HTMLElement;
  const resize = vi.fn();
  const dispose = vi.fn();
  const setOption = vi.fn();
  const chart = { resize, dispose, setOption } as unknown as EChartsType;
  let resizeCallback: (() => void) | undefined;
  let frameCallback: FrameRequestCallback | undefined;
  const observe = vi.fn();
  const disconnect = vi.fn();
  const platform: ChartControllerPlatform = {
    initialize: vi.fn(() => chart),
    createResizeObserver: vi.fn((callback) => {
      resizeCallback = callback;
      return { observe, disconnect };
    }),
    requestFrame: vi.fn((callback) => {
      frameCallback = callback;
      return 7;
    }),
    cancelFrame: vi.fn()
  };
  const sync = vi.fn();
  const controller = new ChartController(host, sync, platform);

  return {
    controller,
    host,
    platform,
    sync,
    chart,
    resize,
    dispose,
    setOption,
    observe,
    disconnect,
    resizeNow: () => resizeCallback?.(),
    runFrame: () => frameCallback?.(0)
  };
}

describe("ChartController", () => {
  it("re-observes and requests a render after reconnecting", () => {
    const fixture = setupController();

    fixture.controller.hostConnected();
    fixture.controller.hostUpdated();
    fixture.controller.hostDisconnected();
    fixture.controller.hostUpdated();
    fixture.controller.hostConnected();
    fixture.controller.hostUpdated();

    expect(fixture.observe).toHaveBeenCalledTimes(2);
    expect(fixture.disconnect).toHaveBeenCalledOnce();
    expect(fixture.host.requestUpdate).toHaveBeenCalledOnce();
    expect(fixture.sync).toHaveBeenCalledTimes(2);
  });

  it("recreates disposed charts and uses batched object-form updates", () => {
    const fixture = setupController();
    const element = {} as HTMLElement;

    fixture.controller.hostConnected();
    fixture.controller.render("primary", element, { animation: false }, "one");
    fixture.controller.render("primary", element, { animation: false }, "one");
    fixture.controller.render("primary", element, { animation: true }, "two");
    fixture.resizeNow();
    fixture.resizeNow();
    fixture.runFrame();

    expect(fixture.platform.initialize).toHaveBeenCalledOnce();
    expect(fixture.setOption).toHaveBeenCalledTimes(2);
    expect(fixture.setOption).toHaveBeenLastCalledWith(
      { animation: true },
      { notMerge: true, lazyUpdate: true, silent: true }
    );
    expect(fixture.platform.requestFrame).toHaveBeenCalledOnce();
    expect(fixture.resize).toHaveBeenCalledTimes(2);

    fixture.controller.hostDisconnected();
    fixture.controller.hostConnected();
    fixture.controller.render("primary", element, { animation: false }, "one");

    expect(fixture.dispose).toHaveBeenCalledOnce();
    expect(fixture.platform.initialize).toHaveBeenCalledTimes(2);
  });
});

describe("deviceResourceSummarySignature", () => {
  it("changes when any rendered resource-summary value changes", () => {
    const original = deviceResourceSummarySignature("Gateway", 10, 20, 3);

    expect(deviceResourceSummarySignature("Gateway 2", 10, 20, 3)).not.toBe(original);
    expect(deviceResourceSummarySignature("Gateway", 11, 20, 3)).not.toBe(original);
    expect(deviceResourceSummarySignature("Gateway", 10, 21, 3)).not.toBe(original);
    expect(deviceResourceSummarySignature("Gateway", 10, 20, 4)).not.toBe(original);
  });
});
