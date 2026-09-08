// Squarified treemap layout (Bruls, Huizing, van Wijk) — lays out items as
// nested rows/columns so cell aspect ratios stay close to square instead of
// degenerating into slivers, which is what a naive slice-and-dice layout
// does once value ranges span more than ~3x.

export interface TreemapItem {
  key: string
  name: string
  value: number
}

export interface TreemapCell {
  item: TreemapItem
  x: number
  y: number
  width: number
  height: number
}

interface Weighted {
  item: TreemapItem
  area: number
}

function worstAspectRatio(row: Weighted[], rowArea: number, shortSide: number): number {
  const rowLength = rowArea / shortSide
  let worst = 0
  for (const { area } of row) {
    const side = area / rowLength
    const ratio = Math.max(rowLength / side, side / rowLength)
    if (ratio > worst) worst = ratio
  }
  return worst
}

export function squarify(
  items: TreemapItem[],
  x: number,
  y: number,
  width: number,
  height: number,
): TreemapCell[] {
  const positive = items.filter((i) => i.value > 0).sort((a, b) => b.value - a.value)
  const total = positive.reduce((sum, i) => sum + i.value, 0)
  if (total <= 0 || width <= 0 || height <= 0) return []

  const scale = (width * height) / total
  let remaining: Weighted[] = positive.map((item) => ({ item, area: item.value * scale }))
  const cells: TreemapCell[] = []

  let rx = x
  let ry = y
  let rw = width
  let rh = height

  while (remaining.length > 0) {
    const shortSide = Math.min(rw, rh)
    let row: Weighted[] = []
    let rowArea = 0
    let bestWorst = Infinity

    for (const next of remaining.slice(0, remaining.length)) {
      if (row.length > 0 && row[row.length - 1] === next) continue
      const candidateRow = [...row, next]
      const candidateArea = rowArea + next.area
      const worst = worstAspectRatio(candidateRow, candidateArea, shortSide)
      if (worst <= bestWorst) {
        row = candidateRow
        rowArea = candidateArea
        bestWorst = worst
      } else {
        break
      }
    }

    const rowLength = rowArea / shortSide
    let offset = 0
    const layoutVertical = rw >= rh // row runs down the left edge, cells stack top-to-bottom

    for (const entry of row) {
      const size = entry.area / rowLength
      if (layoutVertical) {
        cells.push({ item: entry.item, x: rx, y: ry + offset, width: rowLength, height: size })
      } else {
        cells.push({ item: entry.item, x: rx + offset, y: ry, width: size, height: rowLength })
      }
      offset += size
    }

    if (layoutVertical) {
      rx += rowLength
      rw -= rowLength
    } else {
      ry += rowLength
      rh -= rowLength
    }

    remaining = remaining.slice(row.length)
  }

  return cells
}
