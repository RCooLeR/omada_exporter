// Omada Controller 6.3.0.45 Application Analytics light-theme palette, matching
// the controller screenshot even on this card's dark background. Sources:
// /js/constant-Cbo_mtFl.js and /assets/_light-theme-CJPxTmLg.css.
// Omada assigns colors by descending traffic rank, not family ID. Assign them
// to the complete sorted category list before filtering the selected category.
const OMADA_DPI_CATEGORY_COLORS = [
  "#006F71",
  "#00E194",
  "#A6EF00",
  "#0069CB",
  "#F476FF",
  "#87C969",
  "#FFC730",
  "#FF8C27",
  "#E3C893",
  "#69C7EF",
  "#00C9C9",
  "#FC8DCD",
  "#A2AAF7",
  "#7A54FF",
  "#00B2FF",
  "#5576FF",
  "#A6DD56",
  "#4CB033",
  "#CDAB5D",
  "#44B0D7"
] as const;

export function dpiCategoryColor(index: number): string {
  if (!Number.isInteger(index) || index < 0) {
    return "#CCCCCC";
  }

  return OMADA_DPI_CATEGORY_COLORS[index] ?? "#CCCCCC";
}
