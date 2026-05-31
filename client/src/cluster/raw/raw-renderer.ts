import type { ClusterRenderer, ClusterState, RendererOpts } from "../renderer";
import { DEFAULT_PALETTE, type Palette } from "../core/palette";
import { acquireDevice } from "./device";
import instrumentsWGSL from "./passes/instruments.wgsl?raw";
import { SPEC } from "../core/spec";

export class RawClusterRenderer implements ClusterRenderer {
  private device!: GPUDevice;
  private context!: GPUCanvasContext;
  private format!: GPUTextureFormat;
  private palette: Palette = DEFAULT_PALETTE;
  private width = 1;
  private height = 1;
  private pipeline!: GPURenderPipeline;
  private uniformBuf!: GPUBuffer;
  private bindGroup!: GPUBindGroup;

  async init(canvas: HTMLCanvasElement, opts: RendererOpts): Promise<void> {
    const res = await acquireDevice(canvas);
    if (!res.ok) throw new Error(res.reason);
    this.device = res.device;
    this.context = res.context;
    this.format = res.format;
    this.palette = { ...DEFAULT_PALETTE, ...(opts.colors ?? {}) };
    const module = this.device.createShaderModule({ code: instrumentsWGSL });
    this.uniformBuf = this.device.createBuffer({
      size: 128,
      usage: GPUBufferUsage.UNIFORM | GPUBufferUsage.COPY_DST,
    });
    this.pipeline = this.device.createRenderPipeline({
      layout: "auto",
      vertex: { module, entryPoint: "vs" },
      fragment: { module, entryPoint: "fs", targets: [{ format: this.format }] },
      primitive: { topology: "triangle-list" },
    });
    this.bindGroup = this.device.createBindGroup({
      layout: this.pipeline.getBindGroupLayout(0),
      entries: [{ binding: 0, resource: { buffer: this.uniformBuf } }],
    });
  }

  resize(width: number, height: number, dpr: number): void {
    this.width = Math.max(1, Math.round(width * dpr));
    this.height = Math.max(1, Math.round(height * dpr));
  }

  render(state: ClusterState): void {
    const RAD = Math.PI / 180;
    const u = new Float32Array(32);
    u[0] = this.width; u[1] = this.height;
    u[2] = SPEC.sweep.startDeg * RAD; u[3] = SPEC.sweep.extentDeg * RAD;
    u[4] = (state.rpmAngle - SPEC.sweep.startDeg * RAD) / (SPEC.sweep.extentDeg * RAD);
    u[5] = state.redline;
    u[6] = state.speedAngle; u[7] = state.throttle; u[8] = state.brake;
    u[9] = state.gx; u[10] = state.gy;
    const p = this.palette;
    u.set([...p.ringLow, 0], 12);
    u.set([...p.ringMid, 0], 16);
    u.set([...p.ringRed, 0], 20);
    u.set([...p.panel, 1], 24);
    u.set([...p.tick, 1], 28);
    this.device.queue.writeBuffer(this.uniformBuf, 0, u);

    const encoder = this.device.createCommandEncoder();
    const view = this.context.getCurrentTexture().createView();
    const pass = encoder.beginRenderPass({
      colorAttachments: [{ view, clearValue: { r: p.panel[0], g: p.panel[1], b: p.panel[2], a: 1 }, loadOp: "clear", storeOp: "store" }],
    });
    pass.setPipeline(this.pipeline);
    pass.setBindGroup(0, this.bindGroup);
    pass.draw(3);
    pass.end();
    this.device.queue.submit([encoder.finish()]);
  }

  destroy(): void {
    this.device?.destroy?.();
  }
}
