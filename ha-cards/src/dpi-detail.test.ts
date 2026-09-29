import type { TemplateResult } from "lit";
import { describe, expect, it, vi } from "vitest";
import { dpiTrafficShare, formatDpiBytes, renderDpiDetail } from "./dpi-detail";
import { dpiCategoryColor } from "./dpi-colors";
import type { DpiApplicationRecord, DpiCategoryRecord, DpiSiteRecord, DpiSummary } from "./ha-types";

// Inspect Lit's templates without a DOM dependency. This deliberately keeps
// static markup separate from data bindings, including untrusted API labels.
function inspect(value: unknown, templates: TemplateResult[] = [], bindings: unknown[] = []) {
  if (Array.isArray(value)) {
    value.forEach((child) => inspect(child, templates, bindings));
  } else if (value && typeof value === "object" && "_$litType$" in value && "strings" in value && "values" in value) {
    const template = value as TemplateResult;
    templates.push(template);
    template.values.forEach((child) => inspect(child, templates, bindings));
  } else {
    bindings.push(value);
  }
  return { templates, bindings, markup: templates.flatMap((template) => [...template.strings]).join("") };
}

function site(overrides: Partial<DpiSiteRecord> = {}): DpiSiteRecord {
  return { key: "site-home", siteId: "home", site: "Home", totalBytes: 4096, windowSeconds: 86400, categories: [], applications: [], ...overrides };
}

function summary(sites: DpiSiteRecord[]): DpiSummary {
  return { available: true, sites, categories: sites.flatMap((entry) => entry.categories), applications: sites.flatMap((entry) => entry.applications) };
}

function category(index: number): DpiCategoryRecord {
  return { key: `category-${index}`, siteKey: "site-home", site: "Home", siteId: "home", familyId: String(index), name: `Category ${index}`, bytes: index };
}

function application(group: DpiCategoryRecord, index: number, overrides: Partial<DpiApplicationRecord> = {}): DpiApplicationRecord {
  return { ...group, key: `app-${index}`, applicationId: String(index), name: `Application ${index}`, categoryName: group.name, ...overrides };
}

function clickTemplate(template: TemplateResult | undefined): void {
  expect(template).toBeDefined();
  const click = template?.values.find((value): value is () => void => typeof value === "function");
  expect(click).toBeTypeOf("function");
  click?.();
}

function dataRow(view: TemplateResult, name: string): TemplateResult | undefined {
  return inspect(view).templates.find((template) => /^\s*<tr\b/.test(template.strings[0] ?? "") && template.values.includes(name));
}

describe("DPI detail formatting", () => {
  it("distinguishes an observed zero from unavailable traffic", () => {
    expect(formatDpiBytes(0)).toBe("0 B");
    expect(formatDpiBytes(undefined)).toBe("Unavailable");
    expect(formatDpiBytes(1024)).toBe("1.0 KB");
  });

  it("does not invent a percentage when a total is absent or inconsistent", () => {
    expect(dpiTrafficShare(0, 0)).toBe(0);
    expect(dpiTrafficShare(0, 100)).toBe(0);
    expect(dpiTrafficShare(25, 100)).toBe(25);
    expect(dpiTrafficShare(25, undefined)).toBeUndefined();
    expect(dpiTrafficShare(0, undefined)).toBeUndefined();
    expect(dpiTrafficShare(25, 0)).toBeUndefined();
  });

  it("caps only the visual bar, keeping an oversized reported share visible", () => {
    const result = inspect(renderDpiDetail(summary([site({ totalBytes: 10, categories: [{ ...category(1), bytes: 15 }] })])));
    expect(result.bindings).toContain("width: 100%");
    expect(result.bindings).toContain("150.0");
    expect(result.bindings).not.toContain("width: 150%");
  });
});

describe("DPI detail templates", () => {
  it("includes every exported category and application, with no UI row cap", () => {
    const categories = Array.from({ length: 35 }, (_, index) => category(index));
    const applications: DpiApplicationRecord[] = Array.from({ length: 160 }, (_, index) => ({
      ...category(index), key: `app-${index}`, applicationId: String(index), name: `Application ${index}`, categoryName: `Category ${index % 35}`
    }));
    const result = inspect(renderDpiDetail(summary([site({ categories, applications })])));

    for (const row of [...categories, ...applications]) {
      expect(result.bindings).toContain(row.name);
    }
    expect(result.templates.filter((template) => /^\s*<tr\b/.test(template.strings[0] ?? "")).length).toBe(195);
    expect(result.markup).toContain("Application coverage is limited by the bridge's export setting");
    expect(result.markup).toContain("not live throughput");
  });

  it("uses each site's total for shares and keeps independent site sections", () => {
    const result = inspect(renderDpiDetail(summary([
      site({ totalBytes: 100, categories: [{ ...category(1), bytes: 50 }] }),
      site({ key: "office", siteId: "office", site: "Office", totalBytes: 200, categories: [{ ...category(1), bytes: 50 }] })
    ])));
    expect(result.bindings).toContain("DPI traffic for Home");
    expect(result.bindings).toContain("DPI traffic for Office");
    expect(result.bindings).toContain("50.0");
    expect(result.bindings).toContain("25.0");
  });

  it("shows explicit empty tables and unavailable metadata without fake totals", () => {
    const emptySite: DpiSiteRecord = { key: "home", site: "Home", siteId: "home", categories: [], applications: [] };
    const result = inspect(renderDpiDetail(summary([emptySite])));
    expect(result.markup).toContain("No category data exported.");
    expect(result.bindings).toContain("No application data exported.");
    expect(result.bindings.filter((value) => value === "Unavailable")).toHaveLength(2);
    expect(result.bindings).not.toContain("0 B");
  });

  it("keeps malicious site, category and application names in escaped Lit bindings", () => {
    const malicious = '<img src=x onerror="alert(1)"><script>bad()</script>';
    const row = { ...category(1), familyId: malicious, name: malicious };
    const result = inspect(renderDpiDetail(summary([site({
      site: malicious,
      categories: [row],
      applications: [{ ...row, key: "app", applicationId: malicious, categoryName: malicious }]
    })])));
    expect(result.bindings.filter((value) => value === malicious).length).toBeGreaterThanOrEqual(4);
    expect(result.markup).not.toContain(malicious);
    expect(result.markup).not.toContain("<img");
    expect(result.markup).not.toContain("<script");
    // HTML templates, never unsafeHTML/unsafeSVG directive objects for names.
    expect(result.templates.every((template) => template._$litType$ === 1)).toBe(true);
    expect(result.bindings.every((value) => value === null || typeof value !== "object")).toBe(true);
  });
});

describe("DPI category filtering", () => {
  it("uses native category buttons with pressed state and a site-scoped callback", () => {
    const web = category(1);
    const video = category(2);
    const data = summary([site({ categories: [web, video], applications: [application(web, 1), application(video, 2)] })]);
    const onSelect = vi.fn();
    const view = renderDpiDetail(data, new Map([["site-home", video.key]]), onSelect);
    const selected = dataRow(view, video.name);
    const unselected = dataRow(view, web.name);

    expect(selected?.strings.join("")).toContain('<button type="button"');
    expect(selected?.strings.join("")).toContain("aria-pressed=");
    expect(selected?.values).toContain(true);
    expect(unselected?.values).toContain(false);
    const siteTemplate = inspect(view).templates.find((template) => template.strings.join("").includes('aria-label="DPI applications"'))!;
    const countBinding = siteTemplate.strings.findIndex((text) => text.includes("Applications <span>"));
    const labelBinding = siteTemplate.strings.findIndex((text) => text.includes('class="dpi-filter-label"'));
    expect(siteTemplate.values[countBinding]).toBe(1);
    expect(siteTemplate.values[labelBinding]).toBe(video.name);
    clickTemplate(unselected);
    expect(onSelect).toHaveBeenLastCalledWith("site-home", web.key);

    const reset = inspect(view).templates.find((template) => template.strings.join("").includes("All categories") && template.strings.join("").includes("@click="));
    expect(reset?.strings.join("")).toContain('<button type="button"');
    clickTemplate(reset);
    expect(onSelect).toHaveBeenLastCalledWith("site-home", undefined);
  });

  it("matches category identifiers instead of merging same-named categories", () => {
    const first = { ...category(1), name: "Shared label" };
    const second = { ...category(2), name: "Shared label" };
    const data = summary([site({ categories: [first, second], applications: [application(first, 1), application(second, 2)] })]);
    const result = inspect(renderDpiDetail(data, new Map([["site-home", first.key]])));

    expect(result.bindings).toContain("Application 1");
    expect(result.bindings).not.toContain("Application 2");
    expect(result.bindings).toContain(first.name);
  });

  it("falls back to category names only when the selected category lacks an identifier", () => {
    const legacy = { ...category(1), familyId: "", name: "Legacy category" };
    const other = category(2);
    const data = summary([site({
      categories: [legacy, other],
      applications: [application(legacy, 1, { familyId: "42" }), application(other, 2)]
    })]);
    const view = renderDpiDetail(data, new Map([["site-home", legacy.key]]));
    const result = inspect(view);
    expect(result.bindings).toContain("Application 1");
    expect(result.bindings).not.toContain("Application 2");
    expect(dataRow(view, "Application 1")?.values).toContain(`--dpi-color: ${dpiCategoryColor(0)}`);
  });

  it("keeps two sites' category filters independent", () => {
    const first = category(1);
    const second = category(2);
    const officeFirst = { ...first, key: "office-category-1", siteKey: "site-office", siteId: "office", site: "Office" };
    const officeSecond = { ...second, key: "office-category-2", siteKey: "site-office", siteId: "office", site: "Office" };
    const data = summary([
      site({ categories: [first, second], applications: [application(first, 1), application(second, 2)] }),
      site({ key: "site-office", siteId: "office", site: "Office", categories: [officeFirst, officeSecond], applications: [application(officeFirst, 3), application(officeSecond, 4)] })
    ]);
    const result = inspect(renderDpiDetail(data, new Map([["site-home", first.key], ["site-office", officeSecond.key]])));
    expect(result.bindings).toContain("Application 1");
    expect(result.bindings).not.toContain("Application 2");
    expect(result.bindings).not.toContain("Application 3");
    expect(result.bindings).toContain("Application 4");
  });

  it("shows every application if a previously selected category key is missing", () => {
    const first = category(1);
    const second = category(2);
    const data = summary([site({ categories: [first, second], applications: [application(first, 1), application(second, 2)] })]);
    const result = inspect(renderDpiDetail(data, new Map([["site-home", "removed-category"]])));
    expect(result.bindings).toContain("Application 1");
    expect(result.bindings).toContain("Application 2");
  });

  it("shows an explicit empty application state for an available category with no exported apps", () => {
    const first = category(1);
    const second = category(2);
    const result = inspect(renderDpiDetail(summary([site({ categories: [first, second], applications: [application(second, 2)] })]), new Map([["site-home", first.key]])));
    expect(result.bindings).not.toContain("Application 2");
    expect(result.bindings).toContain("No application data exported for this category.");
    expect(result.bindings).toContain(first.name);
    expect(result.templates.filter((template) => /^\s*<tr\b/.test(template.strings[0] ?? "")).length).toBe(3);
  });

  it("preserves site-total share semantics and category colors while filtering", () => {
    const first = { ...category(1), bytes: 700 };
    const second = { ...category(2), bytes: 300 };
    const app = application(second, 2, { bytes: 100 });
    const data = summary([site({ totalBytes: 1000, categories: [first, second], applications: [application(first, 1, { bytes: 600 }), app] })]);
    const allView = renderDpiDetail(data);
    const selectedView = renderDpiDetail(data, new Map([["site-home", second.key]]));
    const filtered = inspect(selectedView);
    expect(filtered.bindings).toContain("10.0");
    expect(filtered.bindings).not.toContain("33.3");
    const allRow = dataRow(allView, app.name);
    const selectedRow = dataRow(selectedView, app.name);
    const color = dpiCategoryColor(1);
    const colorBinding = (template: TemplateResult | undefined) => template?.values.find((value) => typeof value === "string" && value.includes(color));
    expect(color).toMatch(/^#[0-9a-f]{6}$/i);
    expect(colorBinding(allRow)).toBeDefined();
    expect(colorBinding(selectedRow)).toBe(colorBinding(allRow));
    expect(dpiCategoryColor(0)).not.toBe(color);
  });
});
