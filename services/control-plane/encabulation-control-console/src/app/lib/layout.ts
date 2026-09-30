/**
 * Topology layout.
 *
 * Deterministic on purpose. A force simulation would re-settle on every
 * three-second snapshot and the panel would writhe; an operator needs the
 * gateway to be in the same place it was a minute ago. So each node has a fixed
 * grid slot in the registry, and this module turns slots into coordinates and
 * edges into curves.
 *
 * d3 supplies the scales and the curve; the geometry is ours and is pure, which
 * is what makes it testable.
 */

import { curveBumpY, line as d3line, scaleLinear, scaleSqrt } from 'd3'
import type { EdgeView, Health, NodeView } from '@shared/topology'

export const GRID_COLUMNS = 12
export const GRID_ROWS = 6

export interface Box {
  x: number
  y: number
  width: number
  height: number
  cx: number
  cy: number
}

export interface Viewport {
  width: number
  height: number
}

export interface PlacedNode {
  node: NodeView
  box: Box
}

export interface PlacedEdge {
  edge: EdgeView
  /** Cubic control points, in viewport coordinates. */
  from: { x: number; y: number }
  to: { x: number; y: number }
  c1: { x: number; y: number }
  c2: { x: number; y: number }
  /** SVG path for the link. */
  path: string
  /** Stroke width, from the edge rate. */
  width: number
}

/** Plate size as a fraction of one grid cell. */
const PLATE_W = 0.78
const PLATE_H = 0.5

export function layoutNodes(nodes: NodeView[], view: Viewport): PlacedNode[] {
  const cellW = view.width / GRID_COLUMNS
  const cellH = view.height / GRID_ROWS
  const width = cellW * 2 * PLATE_W
  const height = cellH * PLATE_H

  return nodes.map((node) => {
    // `lay.col` is a 12-column slot; plates are two columns wide.
    const cx = (node.lay.col + 1) * cellW
    const cy = (node.lay.row + 0.5) * cellH
    return {
      node,
      box: { x: cx - width / 2, y: cy - height / 2, width, height, cx, cy },
    }
  })
}

const pathOf = d3line<[number, number]>()
  .x((d) => d[0])
  .y((d) => d[1])
  .curve(curveBumpY)

export function layoutEdges(
  edges: EdgeView[],
  placed: PlacedNode[],
  view: Viewport,
): PlacedEdge[] {
  const byName = new Map(placed.map((p) => [p.node.name, p]))
  const widthScale = edgeWidthScale(edges)

  const out: PlacedEdge[] = []
  for (const edge of edges) {
    const source = byName.get(edge.source)
    const target = byName.get(edge.target)
    if (source === undefined || target === undefined) continue

    const down = target.box.cy >= source.box.cy
    const from = {
      x: source.box.cx,
      y: down ? source.box.y + source.box.height : source.box.y,
    }
    const to = { x: target.box.cx, y: down ? target.box.y : target.box.y + target.box.height }
    const span = Math.abs(to.y - from.y)
    const lift = Math.max(18, Math.min(90, span * 0.45))
    const c1 = { x: from.x, y: from.y + (down ? lift : -lift) }
    const c2 = { x: to.x, y: to.y + (down ? -lift : lift) }

    out.push({
      edge,
      from,
      to,
      c1,
      c2,
      path:
        pathOf([
          [from.x, from.y],
          [to.x, to.y],
        ]) ?? `M${from.x},${from.y}L${to.x},${to.y}`,
      width: widthScale(edge.rps ?? 0),
    })
    void view
  }
  return out
}

/** Stroke width by rate: square-root, so a busy edge does not swamp the panel. */
export function edgeWidthScale(edges: EdgeView[]): (rps: number) => number {
  const peak = edges.reduce((max, e) => Math.max(max, e.rps ?? 0), 0)
  return scaleSqrt().domain([0, Math.max(peak, 1)]).range([1, 5]).clamp(true)
}

/** Point on the cubic Bézier at t, used by both the SVG path and the particles. */
export function bezierAt(
  edge: Pick<PlacedEdge, 'from' | 'c1' | 'c2' | 'to'>,
  t: number,
): { x: number; y: number } {
  const u = 1 - t
  const a = u * u * u
  const b = 3 * u * u * t
  const c = 3 * u * t * t
  const d = t * t * t
  return {
    x: a * edge.from.x + b * edge.c1.x + c * edge.c2.x + d * edge.to.x,
    y: a * edge.from.y + b * edge.c1.y + c * edge.c2.y + d * edge.to.y,
  }
}

/** Unit normal at t, for the lateral offset that gives a stream its thickness. */
export function normalAt(
  edge: Pick<PlacedEdge, 'from' | 'c1' | 'c2' | 'to'>,
  t: number,
): { x: number; y: number } {
  const u = 1 - t
  const dx =
    3 * u * u * (edge.c1.x - edge.from.x) +
    6 * u * t * (edge.c2.x - edge.c1.x) +
    3 * t * t * (edge.to.x - edge.c2.x)
  const dy =
    3 * u * u * (edge.c1.y - edge.from.y) +
    6 * u * t * (edge.c2.y - edge.c1.y) +
    3 * t * t * (edge.to.y - edge.c2.y)
  const len = Math.hypot(dx, dy)
  if (len === 0) return { x: 0, y: 0 }
  return { x: -dy / len, y: dx / len }
}

/** The four health colours, as CSS custom properties resolved at paint time. */
export const HEALTH_VAR: Record<Health, string> = {
  healthy: '--c-phosphor',
  degraded: '--c-amber',
  faulted: '--c-lacquer',
  nodata: '--c-slate',
}

export function healthLabel(health: Health): string {
  switch (health) {
    case 'healthy':
      return 'in tolerance'
    case 'degraded':
      return 'off nominal'
    case 'faulted':
      return 'out of tolerance'
    case 'nodata':
      return 'no data'
  }
}

/** Pod dot positions along the bottom edge of a plate, capped and wrapped. */
export function podDots(box: Box, podCount: number, maxPerRow = 10): { x: number; y: number }[] {
  if (podCount <= 0) return []
  const shown = Math.min(podCount, maxPerRow * 2)
  const rows = Math.ceil(shown / maxPerRow)
  const dots: { x: number; y: number }[] = []
  for (let i = 0; i < shown; i += 1) {
    const row = Math.floor(i / maxPerRow)
    const inRow = Math.min(maxPerRow, shown - row * maxPerRow)
    const indexInRow = i % maxPerRow
    const step = box.width / (inRow + 1)
    dots.push({
      x: box.x + step * (indexInRow + 1),
      y: box.y + box.height - 6 - (rows - 1 - row) * 7,
    })
  }
  return dots
}

/** Opacity for a gauge needle band, so a quiet edge fades rather than vanishes. */
export const opacityScale = scaleLinear().domain([0, 1]).range([0.35, 1]).clamp(true)
