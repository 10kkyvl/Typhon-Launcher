import { clampOffset, clampZoom } from './crop';
export const bannerRatio = 4;
export interface BannerView { viewport: number; width: number; height: number; zoom: number; offsetX: number; offsetY: number }
export function bannerScale(v: BannerView): number {
  if (v.viewport <= 0 || v.width <= 0 || v.height <= 0) return 0;
  return Math.max(v.viewport / v.width, v.viewport / bannerRatio / v.height) * clampZoom(v.zoom);
}
export function clampBanner(v: BannerView): BannerView {
  const scale = bannerScale(v);
  return { ...v, offsetX: clampOffset(v.offsetX, v.width * scale, v.viewport), offsetY: clampOffset(v.offsetY, v.height * scale, v.viewport / bannerRatio) };
}
export function zoomBanner(v: BannerView, zoom: number, x = v.viewport / 2, y = v.viewport / bannerRatio / 2): BannerView {
  const next = clampZoom(zoom), ratio = next / clampZoom(v.zoom);
  return clampBanner({ ...v, zoom: next, offsetX: x - (x - v.offsetX) * ratio, offsetY: y - (y - v.offsetY) * ratio });
}
export function bannerCrop(v: BannerView) {
  const scale = bannerScale(v);
  if (!scale) return { x: 0, y: 0, width: 0, height: 0, outputWidth: 0, outputHeight: 0 };
  const width = Math.min(v.viewport / scale, v.width, v.height * bannerRatio), height = width / bannerRatio;
  const outputHeight = Math.max(1, Math.min(640, Math.floor(height)));
  return { x: Math.max(0, Math.min(-v.offsetX / scale, v.width - width)), y: Math.max(0, Math.min(-v.offsetY / scale, v.height - height)), width, height, outputWidth: outputHeight * bannerRatio, outputHeight };
}
