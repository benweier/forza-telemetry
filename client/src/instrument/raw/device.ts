/// <reference types="@webgpu/types" />

export type DeviceResult =
  | { ok: true; device: GPUDevice; context: GPUCanvasContext; format: GPUTextureFormat }
  | { ok: false; reason: string };

export async function acquireDevice(canvas: HTMLCanvasElement): Promise<DeviceResult> {
  if (!("gpu" in navigator)) return { ok: false, reason: "This browser has no WebGPU support." };
  const adapter = await navigator.gpu.requestAdapter();
  if (!adapter) return { ok: false, reason: "No suitable GPU adapter was found." };
  let device: GPUDevice;
  try {
    device = await adapter.requestDevice();
  } catch {
    return { ok: false, reason: "Failed to create a GPU device." };
  }
  const context = canvas.getContext("webgpu");
  if (!context) return { ok: false, reason: "Could not get a WebGPU canvas context." };
  const format = navigator.gpu.getPreferredCanvasFormat();
  context.configure({ device, format, alphaMode: "premultiplied" });
  return { ok: true, device, context, format };
}
