import { describe, expect, it } from "vitest";
import { dpiCategoryColor } from "./dpi-colors";

describe("DPI category colors", () => {
  it("matches all 20 colors in the Omada Application Analytics palette", () => {
    expect(Array.from({ length: 20 }, (_, index) => dpiCategoryColor(index))).toEqual([
      "#006F71", "#00E194", "#A6EF00", "#0069CB", "#F476FF",
      "#87C969", "#FFC730", "#FF8C27", "#E3C893", "#69C7EF",
      "#00C9C9", "#FC8DCD", "#A2AAF7", "#7A54FF", "#00B2FF",
      "#5576FF", "#A6DD56", "#4CB033", "#CDAB5D", "#44B0D7"
    ]);
  });

  it("uses Omada's other-category grey beyond the palette instead of cycling", () => {
    expect(dpiCategoryColor(20)).toBe("#CCCCCC");
    expect(dpiCategoryColor(100)).toBe("#CCCCCC");
  });

  it.each([-1, 0.5, Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY])(
    "uses a safe fallback for invalid rank %s",
    (index) => {
      expect(dpiCategoryColor(index)).toBe("#CCCCCC");
    }
  );

  it("keeps colors assigned to the full ranked list when categories are filtered", () => {
    const categories = Array.from({ length: 3 }, (_, rank) => ({
      familyId: 2085 - rank,
      color: dpiCategoryColor(rank)
    }));

    expect(categories.filter((category) => category.familyId === 2083)[0]?.color).toBe("#A6EF00");
  });
});
