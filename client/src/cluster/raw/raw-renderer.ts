import type { ClusterRenderer, ClusterState, RendererOpts } from "../renderer";
import { DEFAULT_PALETTE, type Palette } from "../core/palette";
import { acquireDevice } from "./device";
import instrumentsWGSL from "./passes/instruments.wgsl?raw";
import bloomWGSL from "./passes/bloom.wgsl?raw";
import { SPEC } from "../core/spec";

const BLOOM_THRESHOLD = 0.6;
const BLOOM_INTENSITY = 0.9;

/** 32-byte uniform buffer layout for bloom passes:
 *  offset 0:  vec2f texel      (8 bytes)
 *  offset 8:  vec2f dir        (8 bytes)
 *  offset 16: f32   threshold  (4 bytes)
 *  offset 20: f32   intensity  (4 bytes)
 *  offset 24: vec2f pad        (8 bytes)
 *  total: 32 bytes
 */
const BLOOM_UBO_SIZE = 32;

export class RawClusterRenderer implements ClusterRenderer {
  private device!: GPUDevice;
  private context!: GPUCanvasContext;
  private format!: GPUTextureFormat;
  private palette: Palette = DEFAULT_PALETTE;
  private width = 1;
  private height = 1;

  // Instrument pass (scene)
  private pipeline!: GPURenderPipeline;
  private uniformBuf!: GPUBuffer;
  private bindGroup!: GPUBindGroup;

  // Bloom offscreen textures
  private sceneTex: GPUTexture | null = null;
  private bloomA: GPUTexture | null = null;
  private bloomB: GPUTexture | null = null;

  // Bloom pipelines (one per entry point so layout:"auto" works per-pass)
  private brightPipeline!: GPURenderPipeline;
  private blurPipeline!: GPURenderPipeline;
  private compositePipeline!: GPURenderPipeline;

  // Shared sampler + bloom uniform buffer
  private bloomSampler!: GPUSampler;
  private bloomUbo!: GPUBuffer;

  async init(canvas: HTMLCanvasElement, opts: RendererOpts): Promise<void> {
    const res = await acquireDevice(canvas);
    if (!res.ok) throw new Error(res.reason);
    this.device = res.device;
    this.context = res.context;
    this.format = res.format;
    this.palette = { ...DEFAULT_PALETTE, ...(opts.colors ?? {}) };

    // ── Instrument pass (renders into sceneTex, rgba16float) ─────────────
    const instrModule = this.device.createShaderModule({ code: instrumentsWGSL });
    this.uniformBuf = this.device.createBuffer({
      size: 128,
      usage: GPUBufferUsage.UNIFORM | GPUBufferUsage.COPY_DST,
    });
    // Fragment target is rgba16float (offscreen), NOT this.format
    this.pipeline = this.device.createRenderPipeline({
      layout: "auto",
      vertex: { module: instrModule, entryPoint: "vs" },
      fragment: { module: instrModule, entryPoint: "fs", targets: [{ format: "rgba16float" }] },
      primitive: { topology: "triangle-list" },
    });
    this.bindGroup = this.device.createBindGroup({
      layout: this.pipeline.getBindGroupLayout(0),
      entries: [{ binding: 0, resource: { buffer: this.uniformBuf } }],
    });

    // ── Bloom shader module ───────────────────────────────────────────────
    const bloomModule = this.device.createShaderModule({ code: bloomWGSL });

    // Shared sampler for all bloom passes
    this.bloomSampler = this.device.createSampler({
      magFilter: "linear",
      minFilter: "linear",
      addressModeU: "clamp-to-edge",
      addressModeV: "clamp-to-edge",
    });

    // Shared uniform buffer for bloom pass parameters
    this.bloomUbo = this.device.createBuffer({
      size: BLOOM_UBO_SIZE,
      usage: GPUBufferUsage.UNIFORM | GPUBufferUsage.COPY_DST,
    });

    // Bright pass: samples sceneTex (tex0), writes bloomA (rgba16float half-res)
    // layout:"auto" — bright entry point doesn't reference tex1, so binding 2 is absent
    this.brightPipeline = this.device.createRenderPipeline({
      layout: "auto",
      vertex: { module: bloomModule, entryPoint: "vs" },
      fragment: { module: bloomModule, entryPoint: "bright", targets: [{ format: "rgba16float" }] },
      primitive: { topology: "triangle-list" },
    });

    // Blur pass: samples one bloom tex (tex0), writes to the other (rgba16float half-res)
    // layout:"auto" — blur entry point doesn't reference tex1, so binding 2 is absent
    this.blurPipeline = this.device.createRenderPipeline({
      layout: "auto",
      vertex: { module: bloomModule, entryPoint: "vs" },
      fragment: { module: bloomModule, entryPoint: "blur", targets: [{ format: "rgba16float" }] },
      primitive: { topology: "triangle-list" },
    });

    // Composite pass: samples sceneTex (tex0) + bloomA (tex1), writes to swapchain
    // layout:"auto" — composite entry point references both tex0 and tex1
    this.compositePipeline = this.device.createRenderPipeline({
      layout: "auto",
      vertex: { module: bloomModule, entryPoint: "vs" },
      fragment: { module: bloomModule, entryPoint: "composite", targets: [{ format: this.format }] },
      primitive: { topology: "triangle-list" },
    });
  }

  resize(width: number, height: number, dpr: number): void {
    this.width = Math.max(1, Math.round(width * dpr));
    this.height = Math.max(1, Math.round(height * dpr));

    // Destroy old offscreen textures
    this.sceneTex?.destroy();
    this.bloomA?.destroy();
    this.bloomB?.destroy();

    const halfW = Math.max(1, Math.floor(this.width / 2));
    const halfH = Math.max(1, Math.floor(this.height / 2));
    const USAGE = GPUTextureUsage.RENDER_ATTACHMENT | GPUTextureUsage.TEXTURE_BINDING;

    this.sceneTex = this.device.createTexture({
      size: [this.width, this.height],
      format: "rgba16float",
      usage: USAGE,
    });
    this.bloomA = this.device.createTexture({
      size: [halfW, halfH],
      format: "rgba16float",
      usage: USAGE,
    });
    this.bloomB = this.device.createTexture({
      size: [halfW, halfH],
      format: "rgba16float",
      usage: USAGE,
    });
  }

  render(state: ClusterState): void {
    // Guard: textures must exist (resize called at least once)
    if (!this.sceneTex || !this.bloomA || !this.bloomB) return;

    // ── Update instrument uniforms ────────────────────────────────────────
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

    const halfW = this.bloomA.width;
    const halfH = this.bloomA.height;

    const encoder = this.device.createCommandEncoder();

    // ── Pass 1: Scene (instrument) → sceneTex ────────────────────────────
    {
      const sceneView = this.sceneTex.createView();
      const pass = encoder.beginRenderPass({
        colorAttachments: [{
          view: sceneView,
          clearValue: { r: p.panel[0], g: p.panel[1], b: p.panel[2], a: 1 },
          loadOp: "clear",
          storeOp: "store",
        }],
      });
      pass.setPipeline(this.pipeline);
      pass.setBindGroup(0, this.bindGroup);
      pass.draw(3);
      pass.end();
    }

    // ── Pass 2: Bright → bloomA (half-res) ───────────────────────────────
    // texel = 1/halfW, 1/halfH (output target pixel size)
    // dir = (0,0), threshold, intensity
    this.writeBloomUbo(1 / halfW, 1 / halfH, 0, 0, BLOOM_THRESHOLD, BLOOM_INTENSITY);
    {
      const brightBG = this.device.createBindGroup({
        layout: this.brightPipeline.getBindGroupLayout(0),
        entries: [
          { binding: 0, resource: this.bloomSampler },
          { binding: 1, resource: this.sceneTex.createView() },
          { binding: 3, resource: { buffer: this.bloomUbo } },
        ],
      });
      const pass = encoder.beginRenderPass({
        colorAttachments: [{
          view: this.bloomA.createView(),
          clearValue: { r: 0, g: 0, b: 0, a: 1 },
          loadOp: "clear",
          storeOp: "store",
        }],
      });
      pass.setPipeline(this.brightPipeline);
      pass.setBindGroup(0, brightBG);
      pass.draw(3);
      pass.end();
    }

    // ── Pass 3: Blur H: bloomA → bloomB (half-res) ───────────────────────
    // dir = (1,0) — horizontal blur
    this.writeBloomUbo(1 / halfW, 1 / halfH, 1, 0, BLOOM_THRESHOLD, BLOOM_INTENSITY);
    {
      const blurHBG = this.device.createBindGroup({
        layout: this.blurPipeline.getBindGroupLayout(0),
        entries: [
          { binding: 0, resource: this.bloomSampler },
          { binding: 1, resource: this.bloomA.createView() },
          { binding: 3, resource: { buffer: this.bloomUbo } },
        ],
      });
      const pass = encoder.beginRenderPass({
        colorAttachments: [{
          view: this.bloomB.createView(),
          clearValue: { r: 0, g: 0, b: 0, a: 1 },
          loadOp: "clear",
          storeOp: "store",
        }],
      });
      pass.setPipeline(this.blurPipeline);
      pass.setBindGroup(0, blurHBG);
      pass.draw(3);
      pass.end();
    }

    // ── Pass 4: Blur V: bloomB → bloomA (half-res) ───────────────────────
    // dir = (0,1) — vertical blur
    this.writeBloomUbo(1 / halfW, 1 / halfH, 0, 1, BLOOM_THRESHOLD, BLOOM_INTENSITY);
    {
      const blurVBG = this.device.createBindGroup({
        layout: this.blurPipeline.getBindGroupLayout(0),
        entries: [
          { binding: 0, resource: this.bloomSampler },
          { binding: 1, resource: this.bloomB.createView() },
          { binding: 3, resource: { buffer: this.bloomUbo } },
        ],
      });
      const pass = encoder.beginRenderPass({
        colorAttachments: [{
          view: this.bloomA.createView(),
          clearValue: { r: 0, g: 0, b: 0, a: 1 },
          loadOp: "clear",
          storeOp: "store",
        }],
      });
      pass.setPipeline(this.blurPipeline);
      pass.setBindGroup(0, blurVBG);
      pass.draw(3);
      pass.end();
    }

    // ── Pass 5: Composite: (sceneTex + bloomA) → swapchain ───────────────
    // texel = 1/width, 1/height (full-res output)
    this.writeBloomUbo(1 / this.width, 1 / this.height, 0, 0, BLOOM_THRESHOLD, BLOOM_INTENSITY);
    {
      const compositeBG = this.device.createBindGroup({
        layout: this.compositePipeline.getBindGroupLayout(0),
        entries: [
          { binding: 0, resource: this.bloomSampler },
          { binding: 1, resource: this.sceneTex.createView() },
          { binding: 2, resource: this.bloomA.createView() },
          { binding: 3, resource: { buffer: this.bloomUbo } },
        ],
      });
      const swapView = this.context.getCurrentTexture().createView();
      const pass = encoder.beginRenderPass({
        colorAttachments: [{
          view: swapView,
          clearValue: { r: 0, g: 0, b: 0, a: 1 },
          loadOp: "clear",
          storeOp: "store",
        }],
      });
      pass.setPipeline(this.compositePipeline);
      pass.setBindGroup(0, compositeBG);
      pass.draw(3);
      pass.end();
    }

    this.device.queue.submit([encoder.finish()]);
  }

  /** Write bloom uniform buffer: texel(x,y), dir(x,y), threshold, intensity, pad(0,0) */
  private writeBloomUbo(
    texelX: number,
    texelY: number,
    dirX: number,
    dirY: number,
    threshold: number,
    intensity: number,
  ): void {
    const data = new Float32Array(8);
    data[0] = texelX;
    data[1] = texelY;
    data[2] = dirX;
    data[3] = dirY;
    data[4] = threshold;
    data[5] = intensity;
    data[6] = 0; // pad
    data[7] = 0; // pad
    this.device.queue.writeBuffer(this.bloomUbo, 0, data);
  }

  destroy(): void {
    this.sceneTex?.destroy();
    this.bloomA?.destroy();
    this.bloomB?.destroy();
    this.device?.destroy?.();
  }
}
