import type { ClusterRenderer, ClusterState, RendererOpts } from "../renderer";
import { DEFAULT_PALETTE, type Palette } from "../core/palette";
import { acquireDevice } from "./device";

export class RawClusterRenderer implements ClusterRenderer {
  private device!: GPUDevice;
  private context!: GPUCanvasContext;
  private format!: GPUTextureFormat;
  private palette: Palette = DEFAULT_PALETTE;
  private width = 1;
  private height = 1;

  async init(canvas: HTMLCanvasElement, opts: RendererOpts): Promise<void> {
    const res = await acquireDevice(canvas);
    if (!res.ok) throw new Error(res.reason);
    this.device = res.device;
    this.context = res.context;
    this.format = res.format;
    this.palette = { ...DEFAULT_PALETTE, ...(opts.colors ?? {}) };
  }

  resize(width: number, height: number, dpr: number): void {
    this.width = Math.max(1, Math.round(width * dpr));
    this.height = Math.max(1, Math.round(height * dpr));
  }

  render(_state: ClusterState): void {
    const [r, g, b] = this.palette.panel;
    const encoder = this.device.createCommandEncoder();
    const view = this.context.getCurrentTexture().createView();
    const pass = encoder.beginRenderPass({
      colorAttachments: [{ view, clearValue: { r, g, b, a: 1 }, loadOp: "clear", storeOp: "store" }],
    });
    pass.end();
    this.device.queue.submit([encoder.finish()]);
  }

  destroy(): void {
    this.device?.destroy?.();
  }
}
