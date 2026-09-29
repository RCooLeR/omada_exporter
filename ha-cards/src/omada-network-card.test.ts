import type { TemplateResult } from "lit";
import { describe, expect, it, vi } from "vitest";
import type { ClientRecord, DashboardModel, DeviceRecord, HassEntity, HomeAssistant } from "./ha-types";

vi.hoisted(() => {
  vi.stubGlobal("window", {});
  vi.stubGlobal("customElements", { get: vi.fn(), define: vi.fn() });
});
vi.mock("./register-card", () => ({ registerCustomCard: vi.fn() }));
import { OmadaNetworkCard } from "./omada-network-card";

type Selection = { kind: "device" | "client" | "dpi"; key: string };
interface CardHarness {
  _model: DashboardModel;
  _selection: Selection | undefined;
  _selectedDevice: DeviceRecord | undefined;
  _selectedClient: ClientRecord | undefined;
  _dpiCategoryFilters: ReadonlyMap<string, string>;
  _chartController: { clear(): void };
  willUpdate(changed: Map<string, unknown>): void;
  renderSummaryChips(): TemplateResult;
  renderDetail(): TemplateResult;
  selectDevice(key: string): void;
  selectClient(key: string): void;
  selectDpiCategory(siteKey: string, categoryKey: string | undefined): void;
  syncCharts(): void;
}

function templates(value: unknown): TemplateResult[] {
  if (Array.isArray(value)) {
    return value.flatMap(templates);
  }
  if (value && typeof value === "object" && "_$litType$" in value && "strings" in value && "values" in value) {
    const template = value as TemplateResult;
    return [template, ...template.values.flatMap(templates)];
  }
  return [];
}

function markup(value: unknown): string {
  return templates(value).flatMap((template) => [...template.strings]).join("");
}

function entity(id: string, state: string, attributes: Record<string, unknown>): HassEntity {
  return { entity_id: id, state, attributes: { site: "Home", ...attributes } };
}

function homeAssistant(dpiState: string | undefined, includeDevice = true, includeClient = true): HomeAssistant {
  const entities: HassEntity[] = [];
  if (includeDevice) {
    entities.push(entity("sensor.gateway_cpu", "42", {
      metric: "omada_device_cpu_percentage", device_mac: "aa:bb:cc:dd:ee:ff", device_name: "Gateway", device_type: "gateway", device_status: "Connected"
    }));
  }
  if (includeClient) {
    entities.push(entity("device_tracker.phone", "home", { mac: "11:22:33:44:55:66", name: "Phone", wireless: "true" }));
  }
  if (dpiState !== undefined) {
    entities.push(entity("sensor.dpi_total", dpiState, { metric: "omada_dpi_total_traffic_bytes", site_id: "home" }));
  }
  return { states: Object.fromEntries(entities.map((entry) => [entry.entity_id, entry])) };
}

function setup(hass = homeAssistant("4096")) {
  // Node uses Lit's SSR HTMLElement shim. Keep the card disconnected and drive
  // its model/selection lifecycle explicitly, without canvas or browser mocks.
  const card = new OmadaNetworkCard();
  const harness = card as unknown as CardHarness;
  card.setConfig({ type: "custom:omada-network-card", site: "Home" });
  const update = (nextHass: HomeAssistant) => {
    card.hass = nextHass;
    harness.willUpdate(new Map([["hass", undefined]]));
  };
  update(hass);
  const clickDpi = () => {
    const button = templates(harness.renderSummaryChips()).find((template) => template.strings.join("").includes('aria-label="Show DPI traffic insights"'));
    expect(button).toBeDefined();
    const click = button?.values.find((value): value is () => void => typeof value === "function");
    expect(click).toBeTypeOf("function");
    click?.();
    harness.willUpdate(new Map([["_selection", undefined]]));
  };
  return { card, harness, update, clickDpi };
}

function categoriesHass(renamed = false): HomeAssistant {
  const hass = homeAssistant("4096");
  for (const siteId of ["home", "office"]) {
    for (const index of [1, 2]) {
      const familyName = renamed && index === 1 ? "Renamed category" : `Category ${index}`;
      const categoryEntity = entity(`sensor.${siteId}_category_${index}`, String(renamed && index === 1 ? 10 : 1000 / index), {
        metric: "omada_dpi_category_traffic_bytes", site_id: siteId, site: "Home", family_id: String(index), family_name: familyName
      });
      const appEntity = entity(`sensor.${siteId}_app_${index}`, "100", {
        metric: "omada_dpi_application_traffic_bytes", site_id: siteId, site: "Home", family_id: String(index), family_name: familyName,
        application_id: String(index), application_name: `${siteId} app ${index}`
      });
      hass.states[categoryEntity.entity_id] = categoryEntity;
      hass.states[appEntity.entity_id] = appEntity;
    }
  }
  return hass;
}

function boundValues(view: TemplateResult): unknown[] {
  return templates(view).flatMap((template) => template.values);
}

function invokeBoundClick(template: TemplateResult | undefined): void {
  expect(template).toBeDefined();
  const click = template?.values.find((value): value is () => void => typeof value === "function");
  expect(click).toBeTypeOf("function");
  click?.();
}

describe("Omada network card DPI selection", () => {
  it.each([undefined, "unknown", "unavailable"])("hides the chip when DPI data is %s", (state) => {
    const fixture = setup(homeAssistant(state));
    expect(markup(fixture.harness.renderSummaryChips())).not.toContain("Show DPI traffic insights");
    expect(fixture.harness._selection?.kind).toBe("device");
  });

  it("keeps the chip available for an explicit zero-byte observation", () => {
    const fixture = setup(homeAssistant("0"));
    expect(markup(fixture.harness.renderSummaryChips())).toContain("Show DPI traffic insights");
    fixture.clickDpi();
    expect(fixture.harness._selection).toEqual({ kind: "dpi", key: "dpi" });
  });

  it("opens DPI in the main detail panel and clears selected records/charts", () => {
    const fixture = setup();
    const clear = vi.spyOn(fixture.harness._chartController, "clear");
    expect(fixture.harness._selectedDevice?.name).toBe("Gateway");
    fixture.clickDpi();
    fixture.harness.syncCharts();
    expect(fixture.harness._selectedDevice).toBeUndefined();
    expect(fixture.harness._selectedClient).toBeUndefined();
    expect(markup(fixture.harness.renderDetail())).toContain("DPI Insights");
    expect(clear).toHaveBeenCalledOnce();
    const button = templates(fixture.harness.renderSummaryChips()).find((template) => template.strings.join("").includes("aria-pressed="));
    expect(button?.values).toContain(true);
    expect(button?.values).toContain("active");
  });

  it("returns to the selected device or client when their list action is invoked", () => {
    const fixture = setup();
    const device = fixture.harness._model.devices[0]!;
    const client = fixture.harness._model.clients[0]!;
    fixture.clickDpi();
    fixture.harness.selectDevice(device.key);
    fixture.harness.willUpdate(new Map([["_selection", undefined]]));
    expect(fixture.harness._selectedDevice).toBe(device);
    expect(markup(fixture.harness.renderDetail())).not.toContain("DPI Insights");
    fixture.clickDpi();
    fixture.harness.selectClient(client.key);
    fixture.harness.willUpdate(new Map([["_selection", undefined]]));
    expect(fixture.harness._selection).toEqual({ kind: "client", key: client.key });
    expect(fixture.harness._selectedClient).toBe(client);
    expect(fixture.harness._selectedDevice).toBeUndefined();
    expect(markup(fixture.harness.renderDetail())).not.toContain("DPI Insights");
  });

  it("preserves the DPI selection while data updates", () => {
    const fixture = setup();
    fixture.clickDpi();
    fixture.update(homeAssistant("8192"));
    expect(fixture.harness._selection?.kind).toBe("dpi");
    expect(fixture.harness._model.dpi.totalBytes).toBe(8192);
    expect(markup(fixture.harness.renderDetail())).toContain("DPI Insights");
  });

  it.each([
    [true, true, "device"],
    [false, true, "client"],
    [false, false, undefined]
  ] as const)("falls back safely when DPI disappears (device=%s, client=%s)", (device, client, kind) => {
    const fixture = setup(homeAssistant("4096", device, client));
    fixture.clickDpi();
    fixture.update(homeAssistant("unavailable", device, client));
    expect(fixture.harness._selection?.kind).toBe(kind);
    expect(markup(fixture.harness.renderSummaryChips())).not.toContain("Show DPI traffic insights");
    expect(markup(fixture.harness.renderDetail())).not.toContain("DPI Insights");
  });

  it("can show DPI when no devices or clients are exported", () => {
    const fixture = setup(homeAssistant("4096", false, false));
    expect(fixture.harness._selection).toEqual({ kind: "dpi", key: "dpi" });
    expect(markup(fixture.harness.renderDetail())).toContain("DPI Insights");
  });
});

describe("Omada network card DPI category selection", () => {
  it("connects category-button and reset callbacks to reactive filter state", () => {
    const fixture = setup(categoriesHass());
    fixture.clickDpi();
    const site = fixture.harness._model.dpi.sites.find((entry) => entry.siteId === "home")!;
    const category = site.categories.find((row) => row.familyId === "1")!;
    const row = templates(fixture.harness.renderDetail()).find((template) =>
      /^\s*<tr\b/.test(template.strings[0] ?? "") && template.values.includes(category.name)
    );
    const originalFilters = fixture.harness._dpiCategoryFilters;
    invokeBoundClick(row);
    fixture.harness.willUpdate(new Map([["_dpiCategoryFilters", originalFilters]]));

    expect(fixture.harness._dpiCategoryFilters).not.toBe(originalFilters);
    expect(fixture.harness._dpiCategoryFilters.get(site.key)).toBe(category.key);
    expect(boundValues(fixture.harness.renderDetail())).toContain("home app 1");
    expect(boundValues(fixture.harness.renderDetail())).not.toContain("home app 2");
    const reset = templates(fixture.harness.renderDetail()).find((template) =>
      template.strings.join("").includes("All categories") && template.strings.join("").includes("@click=")
    );
    invokeBoundClick(reset);
    expect(fixture.harness._dpiCategoryFilters.has(site.key)).toBe(false);
    expect(boundValues(fixture.harness.renderDetail())).toContain("home app 2");
  });

  it("preserves independent site filters and resets only the requested site", () => {
    const fixture = setup(categoriesHass());
    fixture.clickDpi();
    const home = fixture.harness._model.dpi.sites.find((entry) => entry.siteId === "home")!;
    const office = fixture.harness._model.dpi.sites.find((entry) => entry.siteId === "office")!;
    const homeCategory = home.categories.find((row) => row.familyId === "1")!;
    const officeCategory = office.categories.find((row) => row.familyId === "2")!;
    fixture.harness.selectDpiCategory(home.key, homeCategory.key);
    fixture.harness.selectDpiCategory(office.key, officeCategory.key);
    const bindings = boundValues(fixture.harness.renderDetail());
    expect(bindings).toContain("home app 1");
    expect(bindings).not.toContain("home app 2");
    expect(bindings).not.toContain("office app 1");
    expect(bindings).toContain("office app 2");

    fixture.harness.selectDpiCategory(home.key, undefined);
    expect(fixture.harness._dpiCategoryFilters.has(home.key)).toBe(false);
    expect(fixture.harness._dpiCategoryFilters.get(office.key)).toBe(officeCategory.key);
    expect(boundValues(fixture.harness.renderDetail())).toContain("home app 2");
    expect(boundValues(fixture.harness.renderDetail())).not.toContain("office app 1");
  });

  it("preserves a stable category key when name, bytes, and traffic rank change", () => {
    const fixture = setup(categoriesHass());
    fixture.clickDpi();
    const site = fixture.harness._model.dpi.sites.find((entry) => entry.siteId === "home")!;
    const selected = site.categories.find((row) => row.familyId === "1")!;
    fixture.harness.selectDpiCategory(site.key, selected.key);
    fixture.update(categoriesHass(true));

    const updatedSite = fixture.harness._model.dpi.sites.find((entry) => entry.siteId === "home")!;
    expect(updatedSite.categories[0]?.familyId).toBe("2");
    expect(fixture.harness._dpiCategoryFilters.get(site.key)).toBe(selected.key);
    const bindings = boundValues(fixture.harness.renderDetail());
    expect(bindings).toContain("Renamed category");
    expect(bindings).toContain("home app 1");
    expect(bindings).not.toContain("home app 2");
  });

  it("prunes a removed category or site without affecting other valid selections", () => {
    const fixture = setup(categoriesHass());
    fixture.clickDpi();
    const home = fixture.harness._model.dpi.sites.find((entry) => entry.siteId === "home")!;
    const office = fixture.harness._model.dpi.sites.find((entry) => entry.siteId === "office")!;
    const homeCategory = home.categories.find((row) => row.familyId === "1")!;
    const officeCategory = office.categories.find((row) => row.familyId === "2")!;
    fixture.harness.selectDpiCategory(home.key, homeCategory.key);
    fixture.harness.selectDpiCategory(office.key, officeCategory.key);

    const removedCategory = categoriesHass();
    delete removedCategory.states["sensor.home_category_1"];
    fixture.update(removedCategory);
    expect(fixture.harness._dpiCategoryFilters.has(home.key)).toBe(false);
    expect(fixture.harness._dpiCategoryFilters.get(office.key)).toBe(officeCategory.key);
    expect(boundValues(fixture.harness.renderDetail())).toContain("home app 2");

    const removedSite: HomeAssistant = {
      states: Object.fromEntries(Object.entries(removedCategory.states).filter(([, value]) => value.attributes.site_id !== "office"))
    };
    fixture.update(removedSite);
    expect(fixture.harness._dpiCategoryFilters.size).toBe(0);
    expect(fixture.harness._selection?.kind).toBe("dpi");
  });
});
