import type { PreviewLayout } from "./protocol";

export function builtInLayouts(): PreviewLayout[] {
  return [
    createGridLayout("grid-8x8", "Grid 8x8", 8, 8, 1),
    createGridLayout("grid-32x32", "Grid 32x32", 32, 32, 1),
    createLineLayout("line-32", "Line 32", 32, 1),
    createDiamondLayout("diamond-25", "Diamond 25", 5, 1)
  ];
}

function createLineLayout(id: string, name: string, count: number, spacing: number): PreviewLayout {
  const points = [];
  for (let index = 0; index < count; index += 1) {
    points.push({ index, x: index * spacing, y: 0 });
  }
  return { id, name, width: Math.max(1, count), height: 1, points };
}

function createGridLayout(id: string, name: string, width: number, height: number, spacing: number): PreviewLayout {
  const points = [];
  let index = 0;
  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      points.push({ index, x: x * spacing, y: y * spacing });
      index += 1;
    }
  }
  return { id, name, width: Math.max(1, width), height: Math.max(1, height), points };
}

function createDiamondLayout(id: string, name: string, radius: number, spacing: number): PreviewLayout {
  const points = [];
  let index = 0;
  for (let y = -radius + 1; y < radius; y += 1) {
    for (let x = -radius + 1; x < radius; x += 1) {
      if (Math.abs(x) + Math.abs(y) >= radius) {
        continue;
      }
      points.push({
        index,
        x: (x + radius - 1) * spacing,
        y: (y + radius - 1) * spacing
      });
      index += 1;
    }
  }
  return { id, name, width: Math.max(1, radius * 2 - 1), height: Math.max(1, radius * 2 - 1), points };
}
