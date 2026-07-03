import type { InstrumentRenderer, InstrumentState, RendererOpts } from "../renderer";
import { DEFAULT_PALETTE, type Palette } from "../core/palette";
import { acquireDevice } from "./device";
import instrumentsWGSL from "./passes/instruments.wgsl?raw";
import bloomWGSL from "./passes/bloom.wgsl?raw";
import glyphsWGSL from "./passes/glyphs.wgsl?raw";
import atlasJson from "../core/msdf/atlas.json";
import atlasUrl from "../core/msdf/atlas.png";
import { buildGlyphMap, layoutText, measureText, type RawAtlas } from "../core/msdf/layout";
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

/** Max glyphs in the dynamic vertex buffer. 6 verts per glyph, 32 bytes per vert. */
const MAX_GLYPHS = 64;
const BYTES_PER_VERT = 32; // posClip(2*4) + uv(2*4) + color(4*4) = 32

export class RawInstrumentRenderer implements InstrumentRenderer {
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

  // Shared sampler + per-pass bloom uniform buffers
  private bloomSampler!: GPUSampler;
  private brightUBO!: GPUBuffer;
  private blurHUBO!: GPUBuffer;
  private blurVUBO!: GPUBuffer;
  private compositeUBO!: GPUBuffer;

  // Text pass
  private glyphPipeline!: GPURenderPipeline;
  private glyphSampler!: GPUSampler;
  private atlasTex!: GPUTexture;
  private glyphBindGroup!: GPUBindGroup;
  private glyphVertexBuf!: GPUBuffer;
  private glyphMap!: Map<string, import("../core/msdf/layout").RawGlyph>;
  private atlasCommon!: RawAtlas["common"];

  async init(canvas: HTMLCanvasElement, opts: RendererOpts): Promise<void> {
    const res = await acquireDevice(canvas);
    if (!res.ok) throw new Error(res.reason);
    this.device = res.device;
    this.context = res.context;
    this.format = res.format;
    this.palette = { ...DEFAULT_PALETTE, ...opts.colors };

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

    // Per-pass uniform buffers — each pass gets its own so no aliasing occurs
    // within a single submit where all passes execute after the final writeBuffer.
    const uboUsage = GPUBufferUsage.UNIFORM | GPUBufferUsage.COPY_DST;
    this.brightUBO    = this.device.createBuffer({ size: BLOOM_UBO_SIZE, usage: uboUsage });
    this.blurHUBO     = this.device.createBuffer({ size: BLOOM_UBO_SIZE, usage: uboUsage });
    this.blurVUBO     = this.device.createBuffer({ size: BLOOM_UBO_SIZE, usage: uboUsage });
    this.compositeUBO = this.device.createBuffer({ size: BLOOM_UBO_SIZE, usage: uboUsage });

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

    // ── Text (MSDF) pass ─────────────────────────────────────────────────
    const rawAtlas: RawAtlas = atlasJson;
    this.glyphMap = buildGlyphMap(rawAtlas);
    this.atlasCommon = rawAtlas.common;

    // Load atlas PNG into GPUTexture
    const img = await createImageBitmap(await (await fetch(atlasUrl)).blob());
    this.atlasTex = this.device.createTexture({
      size: [img.width, img.height],
      format: "rgba8unorm",
      usage:
        GPUTextureUsage.TEXTURE_BINDING |
        GPUTextureUsage.COPY_DST |
        GPUTextureUsage.RENDER_ATTACHMENT,
    });
    this.device.queue.copyExternalImageToTexture(
      { source: img },
      { texture: this.atlasTex },
      [img.width, img.height],
    );

    // Linear sampler for atlas
    this.glyphSampler = this.device.createSampler({
      magFilter: "linear",
      minFilter: "linear",
      addressModeU: "clamp-to-edge",
      addressModeV: "clamp-to-edge",
    });

    // Text render pipeline: interleaved vertex buffer, alpha blending over swapchain
    const glyphModule = this.device.createShaderModule({ code: glyphsWGSL });
    this.glyphPipeline = this.device.createRenderPipeline({
      layout: "auto",
      vertex: {
        module: glyphModule,
        entryPoint: "vs",
        buffers: [
          {
            arrayStride: BYTES_PER_VERT,
            attributes: [
              { shaderLocation: 0, offset: 0,  format: "float32x2" }, // posClip
              { shaderLocation: 1, offset: 8,  format: "float32x2" }, // uv
              { shaderLocation: 2, offset: 16, format: "float32x4" }, // color
            ],
          },
        ],
      },
      fragment: {
        module: glyphModule,
        entryPoint: "fs",
        targets: [
          {
            format: this.format,
            blend: {
              color: { srcFactor: "src-alpha", dstFactor: "one-minus-src-alpha", operation: "add" },
              alpha: { srcFactor: "one",       dstFactor: "one-minus-src-alpha", operation: "add" },
            },
          },
        ],
      },
      primitive: { topology: "triangle-list" },
    });

    this.glyphBindGroup = this.device.createBindGroup({
      layout: this.glyphPipeline.getBindGroupLayout(0),
      entries: [
        { binding: 0, resource: this.glyphSampler },
        { binding: 1, resource: this.atlasTex.createView() },
      ],
    });

    // Dynamic vertex buffer: MAX_GLYPHS * 6 verts * BYTES_PER_VERT
    this.glyphVertexBuf = this.device.createBuffer({
      size: MAX_GLYPHS * 6 * BYTES_PER_VERT,
      usage: GPUBufferUsage.VERTEX | GPUBufferUsage.COPY_DST,
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

  render(state: InstrumentState): void {
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
    // texel = 1/halfW, 1/halfH (output target pixel size); dir unused (0,0)
    this.writeBloomUbo(this.brightUBO, 1 / halfW, 1 / halfH, 0, 0, BLOOM_THRESHOLD, 1.0);
    {
      const brightBG = this.device.createBindGroup({
        layout: this.brightPipeline.getBindGroupLayout(0),
        entries: [
          { binding: 0, resource: this.bloomSampler },
          { binding: 1, resource: this.sceneTex.createView() },
          { binding: 3, resource: { buffer: this.brightUBO } },
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
    // texel = 1/halfW, 1/halfH; dir = (1,0) — horizontal blur
    this.writeBloomUbo(this.blurHUBO, 1 / halfW, 1 / halfH, 1, 0, 0, 0);
    {
      const blurHBG = this.device.createBindGroup({
        layout: this.blurPipeline.getBindGroupLayout(0),
        entries: [
          { binding: 0, resource: this.bloomSampler },
          { binding: 1, resource: this.bloomA.createView() },
          { binding: 3, resource: { buffer: this.blurHUBO } },
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
    // texel = 1/halfW, 1/halfH; dir = (0,1) — vertical blur
    this.writeBloomUbo(this.blurVUBO, 1 / halfW, 1 / halfH, 0, 1, 0, 0);
    {
      const blurVBG = this.device.createBindGroup({
        layout: this.blurPipeline.getBindGroupLayout(0),
        entries: [
          { binding: 0, resource: this.bloomSampler },
          { binding: 1, resource: this.bloomB.createView() },
          { binding: 3, resource: { buffer: this.blurVUBO } },
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
    // texel = 1/width, 1/height (full-res output); intensity = BLOOM_INTENSITY; dir unused
    this.writeBloomUbo(this.compositeUBO, 1 / this.width, 1 / this.height, 0, 0, 0, BLOOM_INTENSITY);
    {
      const compositeBG = this.device.createBindGroup({
        layout: this.compositePipeline.getBindGroupLayout(0),
        entries: [
          { binding: 0, resource: this.bloomSampler },
          { binding: 1, resource: this.sceneTex.createView() },
          { binding: 2, resource: this.bloomA.createView() },
          { binding: 3, resource: { buffer: this.compositeUBO } },
        ],
      });
      const swapTex = this.context.getCurrentTexture();
      const swapView = swapTex.createView();
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

      // ── Pass 6: Text (MSDF) drawn over composite (loadOp:"load") ─────────
      const textVerts = this.buildTextVerts(state);
      if (textVerts.vertCount > 0) {
        this.device.queue.writeBuffer(this.glyphVertexBuf, 0, textVerts.data, 0, textVerts.vertCount * (BYTES_PER_VERT / 4));
        const textPass = encoder.beginRenderPass({
          colorAttachments: [{
            view: swapTex.createView(),
            loadOp: "load",
            storeOp: "store",
          }],
        });
        textPass.setPipeline(this.glyphPipeline);
        textPass.setBindGroup(0, this.glyphBindGroup);
        textPass.setVertexBuffer(0, this.glyphVertexBuf);
        textPass.draw(textVerts.vertCount);
        textPass.end();
      }
    }

    this.device.queue.submit([encoder.finish()]);
  }

  /**
   * Build a Float32Array of interleaved vertex data for all text labels.
   * Each vertex: posClip(x,y), uv(u,v), color(r,g,b,a) — 8 floats = 32 bytes.
   * Each glyph quad = 6 vertices (2 triangles).
   *
   * Coordinate conventions:
   *   - anchorNX/anchorNY are 0..1 fractions of canvas device-px dims.
   *   - Device px: anchorX = anchorNX * this.width, anchorY = anchorNY * this.height.
   *   - Clip: xClip = (px / this.width)  * 2 - 1
   *           yClip = 1 - (py / this.height) * 2   (Y flipped: GPU clip +y is up)
   *   - fontPx sets the rendered em-height relative to atlas lineHeight.
   *   - scale = fontPx / atlasCommon.lineHeight
   *   - Text is centered horizontally at anchor; baselines placed so the
   *     top of the line sits at anchorY - fontPx*0.5 (i.e. anchor is the
   *     vertical middle of the run).
   */
  private buildTextVerts(state: InstrumentState): { data: Float32Array; vertCount: number } {
    const floatsPerVert = BYTES_PER_VERT / 4; // 8
    const data = new Float32Array(MAX_GLYPHS * 6 * floatsPerVert);
    let vertCount = 0;

    const mn = Math.min(this.width, this.height);

    const appendRun = (
      text: string,
      anchorNX: number,
      anchorNY: number,
      fontPx: number,
      color: [number, number, number, number],
      align: "center" | "left" = "center",
    ) => {
      const scale = fontPx / this.atlasCommon.lineHeight;
      const quads = layoutText(text, this.glyphMap, { common: this.atlasCommon, chars: [] }, scale);
      const totalWidth = measureText(text, this.glyphMap, scale);

      // Anchor in device px
      const anchorX = anchorNX * this.width;
      const anchorY = anchorNY * this.height;

      // Horizontal start: center or left-align
      const startX = align === "center" ? anchorX - totalWidth * 0.5 : anchorX;
      // Vertical: place so the middle of the glyph run (lineHeight/2 from top) sits at anchorY
      const startY = anchorY - (this.atlasCommon.lineHeight * scale) * 0.5;

      for (const q of quads) {
        if (vertCount + 6 > MAX_GLYPHS * 6) break;

        // Quad corners in device px (Y down)
        const x0 = startX + q.x;
        const y0 = startY + q.y;
        const x1 = x0 + q.w;
        const y1 = y0 + q.h;

        // Clip space (Y up)
        const cx0 = (x0 / this.width)  * 2 - 1;
        const cy0 = 1 - (y0 / this.height) * 2;
        const cx1 = (x1 / this.width)  * 2 - 1;
        const cy1 = 1 - (y1 / this.height) * 2;

        const [r, g, b, a] = color;

        // Two triangles: TL, TR, BL, TR, BR, BL
        const verts: Array<[number, number, number, number]> = [
          [cx0, cy0, q.u0, q.v0], // TL
          [cx1, cy0, q.u1, q.v0], // TR
          [cx0, cy1, q.u0, q.v1], // BL
          [cx1, cy0, q.u1, q.v0], // TR
          [cx1, cy1, q.u1, q.v1], // BR
          [cx0, cy1, q.u0, q.v1], // BL
        ];

        for (const [px, py, u, v] of verts) {
          const off = vertCount * floatsPerVert;
          data[off + 0] = px;
          data[off + 1] = py;
          data[off + 2] = u;
          data[off + 3] = v;
          data[off + 4] = r;
          data[off + 5] = g;
          data[off + 6] = b;
          data[off + 7] = a;
          vertCount++;
        }
      }
    };

    const p = this.palette;

    // Speed — big, centered at gauge center, slightly below middle
    appendRun(
      String(Math.round(state.speedKmh)),
      SPEC.gauge.cx,
      0.56,
      mn * 0.13,
      [...p.textPrimary, 1] as [number, number, number, number],
    );

    // "KM/H" — small, centered under speed
    appendRun(
      "KM/H",
      SPEC.gauge.cx,
      0.66,
      mn * 0.035,
      [...p.textMuted, 1] as [number, number, number, number],
    );

    // RPM — small, centered above gauge middle
    appendRun(
      String(Math.round(state.rpm)),
      SPEC.gauge.cx,
      0.34,
      mn * 0.035,
      [...p.rpmCaption, 1] as [number, number, number, number],
    );

    // Gear — large, in gear tile
    appendRun(
      state.gear,
      SPEC.rail.x,
      SPEC.rail.gear.cy,
      mn * 0.09,
      [...p.textPrimary, 1] as [number, number, number, number],
    );

    return { data, vertCount };
  }

  /** Write a per-pass bloom uniform buffer: texel(x,y), dir(x,y), threshold, intensity, pad(0,0) */
  private writeBloomUbo(
    ubo: GPUBuffer,
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
    this.device.queue.writeBuffer(ubo, 0, data);
  }

  destroy(): void {
    this.sceneTex?.destroy();
    this.bloomA?.destroy();
    this.bloomB?.destroy();
    this.brightUBO?.destroy();
    this.blurHUBO?.destroy();
    this.blurVUBO?.destroy();
    this.compositeUBO?.destroy();
    this.atlasTex?.destroy();
    this.glyphVertexBuf?.destroy();
    this.device?.destroy?.();
  }
}
