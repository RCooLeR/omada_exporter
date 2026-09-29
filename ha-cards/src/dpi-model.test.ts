import { describe, expect, it } from "vitest";
import type { HassEntity, HomeAssistant } from "./ha-types";
import { buildDashboardModel, cardHassChanged } from "./model";

function dpiEntity(
  id: string,
  metric: string,
  state: string,
  attributes: Record<string, unknown> = {},
  updatedAt = "2026-09-07T10:00:00Z"
): HassEntity {
  return {
    entity_id: `sensor.${id}`,
    state,
    last_updated: updatedAt,
    attributes: { metric: `omada_dpi_${metric}`, site: "Home", site_id: "site-home", ...attributes }
  };
}

function model(entities: HassEntity[], site?: string) {
  const hass: HomeAssistant = { states: Object.fromEntries(entities.map((entity) => [entity.entity_id, entity])) };
  return buildDashboardModel(hass, site).dpi;
}

describe("DPI dashboard model", () => {
  it("groups all supported metrics and sorts traffic rows by bytes", () => {
    const result = model([
      dpiEntity("total", "total_traffic_bytes", "4096"),
      dpiEntity("window", "scrape_window_seconds", "86400"),
      dpiEntity("streaming", "category_traffic_bytes", "3072", { family_id: "1", family_name: "Streaming" }),
      dpiEntity("web", "category_traffic_bytes", "1024", { family_id: "2", family_name: "Web" }),
      dpiEntity("browser", "application_traffic_bytes", "512", {
        family_id: "2", family_name: "Web", application_id: "10", application_name: "Browser"
      }),
      dpiEntity("video", "application_traffic_bytes", "2048", {
        family_id: "1", family_name: "Streaming", application_id: "11", application_name: "Video"
      })
    ]);

    expect(result.available).toBe(true);
    expect(result.totalBytes).toBe(4096);
    expect(result.sites).toHaveLength(1);
    expect(result.sites[0]).toMatchObject({ site: "Home", siteId: "site-home", totalBytes: 4096, windowSeconds: 86400 });
    expect(result.categories.map((row) => row.name)).toEqual(["Streaming", "Web"]);
    expect(result.applications.map((row) => row.name)).toEqual(["Video", "Browser"]);
    expect(result.applications[0]).toMatchObject({ applicationId: "11", familyId: "1", categoryName: "Streaming" });
  });

  it("deduplicates by site and numeric identifiers rather than mutable names", () => {
    const old = dpiEntity("old", "application_traffic_bytes", "999", {
      family_id: 2, family_name: "Old category", application_id: 10, application_name: "Old app"
    }, "2026-09-07T09:00:00Z");
    const latest = dpiEntity("latest", "application_traffic_bytes", "100", {
      site: "Renamed Home", family_id: "2", family_name: "Web", application_id: "10", application_name: "Browser"
    });
    const result = model([latest, old]);

    expect(result.applications).toHaveLength(1);
    expect(result.applications[0]).toMatchObject({ name: "Browser", categoryName: "Web", bytes: 100, site: "Renamed Home" });
    expect(result.applications[0]?.key).toBe(model([old]).applications[0]?.key);
  });

  it("keeps same-named sites and identical category/app identifiers separate", () => {
    const entities = ["a", "b"].flatMap((siteId, index) => [
      dpiEntity(`total_${siteId}`, "total_traffic_bytes", String(100 * (index + 1)), { site_id: siteId }),
      dpiEntity(`category_${siteId}`, "category_traffic_bytes", String(50 * (index + 1)), {
        site_id: siteId, family_id: "1", family_name: "Web"
      }),
      dpiEntity(`app_${siteId}`, "application_traffic_bytes", String(25 * (index + 1)), {
        site_id: siteId, family_id: "1", family_name: "Web", application_id: "1", application_name: "Browser"
      })
    ]);
    const result = model(entities);

    expect(result.sites).toHaveLength(2);
    expect(result.totalBytes).toBe(300);
    expect(new Set(result.categories.map((row) => row.key)).size).toBe(2);
    expect(new Set(result.applications.map((row) => row.key)).size).toBe(2);
  });

  it("supports site-name identity on older sensors and respects the card site filter", () => {
    const entities = [
      dpiEntity("home", "total_traffic_bytes", "10", { site_id: "" }),
      dpiEntity("office", "total_traffic_bytes", "20", { site_id: "", site: "Office" })
    ];

    expect(model(entities).sites).toHaveLength(2);
    expect(model(entities, "Home").totalBytes).toBe(10);
    expect(model(entities, "Office").sites.map((site) => site.site)).toEqual(["Office"]);
    expect(model(entities, "Other").available).toBe(false);
  });

  it.each(["unknown", "unavailable", "", " ", "NaN", "Infinity", "-Infinity", "-1", "on", "off"])(
    "does not turn invalid state %j into a traffic value",
    (state) => {
      const result = model([
        dpiEntity("total", "total_traffic_bytes", state),
        dpiEntity("category", "category_traffic_bytes", state, { family_id: "1" }),
        dpiEntity("app", "application_traffic_bytes", state, { family_id: "1", application_id: "1" }),
        dpiEntity("window", "scrape_window_seconds", "86400")
      ]);

      expect(result).toEqual({ available: false, sites: [], categories: [], applications: [] });
    }
  );

  it("accepts explicit zero traffic, but not a window or unknown metric alone", () => {
    expect(model([dpiEntity("total", "total_traffic_bytes", "0")])).toMatchObject({ available: true, totalBytes: 0 });
    expect(model([dpiEntity("window", "scrape_window_seconds", "86400")]).available).toBe(false);
    expect(model([dpiEntity("other", "future_metric", "10")]).available).toBe(false);
    expect(model([]).available).toBe(false);
  });

  it("lets a newer unavailable state invalidate older retained duplicates", () => {
    const attrs = { family_id: "1", family_name: "Web", last_updated: "2026-09-07T09:00:00Z" };
    const older = dpiEntity("old", "category_traffic_bytes", "123", attrs, "2026-09-07T09:00:00Z");
    const unavailable = dpiEntity("new", "category_traffic_bytes", "unavailable", attrs);

    expect(model([older, unavailable]).available).toBe(false);
    expect(model([unavailable, older]).available).toBe(false);
  });

  it("drops retained rows from earlier MQTT snapshots without confusing HA update timestamps", () => {
    const now = { last_updated: "2026-09-07T10:00:00Z" };
    const before = { last_updated: "2026-09-07T09:00:00Z" };
    const result = model([
      dpiEntity("total", "total_traffic_bytes", "100", now),
      dpiEntity("window", "scrape_window_seconds", "86400", now, "2026-09-07T10:00:02Z"),
      dpiEntity("old_category", "category_traffic_bytes", "1000", { ...before, family_id: "1" }),
      dpiEntity("old_app", "application_traffic_bytes", "1000", { ...before, family_id: "1", application_id: "1" }),
      dpiEntity("category", "category_traffic_bytes", "80", { ...now, family_id: "2" }, "2026-09-07T10:00:03Z"),
      dpiEntity("app", "application_traffic_bytes", "50", { ...now, family_id: "2", application_id: "2" }, "2026-09-07T10:00:05Z")
    ]);

    expect(result.sites[0]?.observedAt).toBe("2026-09-07T10:00:00.000Z");
    expect(result.categories).toHaveLength(1);
    expect(result.applications).toHaveLength(1);
    expect(result.categories[0]?.bytes).toBe(80);
    expect(result.applications[0]?.bytes).toBe(50);
  });

  it("does not resurrect a prior total when a newer snapshot has no valid traffic", () => {
    expect(model([
      dpiEntity("old_total", "total_traffic_bytes", "100", { last_updated: "2026-09-07T09:00:00Z" }),
      dpiEntity("window", "scrape_window_seconds", "86400", { last_updated: "2026-09-07T10:00:00Z" })
    ]).available).toBe(false);
  });

  it("does not claim a full total from partial sites, category totals, or capped applications", () => {
    const result = model([
      dpiEntity("total", "total_traffic_bytes", "100"),
      dpiEntity("app", "application_traffic_bytes", "50", {
        site: "Office", site_id: "office", family_id: "1", application_id: "1"
      })
    ]);

    expect(result.available).toBe(true);
    expect(result.totalBytes).toBeUndefined();
    expect(result.sites[1]?.totalBytes).toBeUndefined();
  });

  it("preserves every exported application without a second display cap", () => {
    const entities = Array.from({ length: 75 }, (_, index) => dpiEntity(`app_${index}`, "application_traffic_bytes", String(index), {
      family_id: "1", family_name: "Web", application_id: String(index), application_name: `App ${index}`
    }));

    expect(model(entities).applications).toHaveLength(75);
    expect(model(entities).applications[0]?.bytes).toBe(74);
  });

  it("does not expose an overflowing sum as Infinity", () => {
    const result = model([
      dpiEntity("home", "total_traffic_bytes", "1e308"),
      dpiEntity("office", "total_traffic_bytes", "1e308", { site: "Office", site_id: "office" })
    ]);

    expect(result.available).toBe(true);
    expect(result.totalBytes).toBeUndefined();
  });

  it("invalidates rendering when a DPI entity changes", () => {
    const original = dpiEntity("total", "total_traffic_bytes", "10");
    const previous: HomeAssistant = { states: { [original.entity_id]: original } };
    const next: HomeAssistant = { states: { [original.entity_id]: { ...original, state: "20" } } };

    expect(cardHassChanged(next, previous)).toBe(true);
  });
});
