export const outputSize = 512;
export const minZoom = 1;
export const maxZoom = 4;

export interface CropView {
  viewport: number;
  width: number;
  height: number;
  zoom: number;
  offsetX: number;
  offsetY: number;
}

export interface CropSource {
  sx: number;
  sy: number;
  size: number;
}

export function coverScale(viewport: number, width: number, height: number): number {
  if (viewport <= 0 || width <= 0 || height <= 0) return 0;
  return viewport / Math.min(width, height);
}

export function clampZoom(zoom: number): number {
  if (!Number.isFinite(zoom)) return minZoom;
  return Math.min(maxZoom, Math.max(minZoom, zoom));
}

export function clampOffset(offset: number, displayed: number, viewport: number): number {
  if (displayed <= viewport) return (viewport - displayed) / 2;
  return Math.min(0, Math.max(viewport - displayed, offset));
}

export function centerOffset(displayed: number, viewport: number): number {
  return (viewport - displayed) / 2;
}

export function zoomAround(view: CropView, zoom: number, anchorX: number, anchorY: number): CropView {
  const scale = coverScale(view.viewport, view.width, view.height);
  if (scale <= 0) return view;

  const next = clampZoom(zoom);
  const ratio = next / view.zoom;
  const offsetX = anchorX - (anchorX - view.offsetX) * ratio;
  const offsetY = anchorY - (anchorY - view.offsetY) * ratio;
  return {
    ...view,
    zoom: next,
    offsetX: clampOffset(offsetX, view.width * scale * next, view.viewport),
    offsetY: clampOffset(offsetY, view.height * scale * next, view.viewport),
  };
}

export function cropSource(view: CropView): CropSource {
  const scale = coverScale(view.viewport, view.width, view.height) * view.zoom;
  if (scale <= 0) return { sx: 0, sy: 0, size: 0 };

  const size = Math.min(view.viewport / scale, view.width, view.height);
  return {
    sx: bound(-view.offsetX / scale, view.width - size),
    sy: bound(-view.offsetY / scale, view.height - size),
    size,
  };
}

function bound(value: number, limit: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(Math.max(value, 0), Math.max(limit, 0));
}
