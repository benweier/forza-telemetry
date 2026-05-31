struct VOut {
  @builtin(position) pos: vec4f,
  @location(0) uv: vec2f,
  @location(1) color: vec4f
};

@vertex fn vs(
  @location(0) posClip: vec2f,
  @location(1) uv: vec2f,
  @location(2) color: vec4f,
) -> VOut {
  var o: VOut;
  o.pos = vec4f(posClip, 0., 1.);
  o.uv = uv;
  o.color = color;
  return o;
}

@group(0) @binding(0) var samp: sampler;
@group(0) @binding(1) var atlas: texture_2d<f32>;

fn median(v: vec3f) -> f32 {
  return max(min(v.r, v.g), min(max(v.r, v.g), v.b));
}

@fragment fn fs(in: VOut) -> @location(0) vec4f {
  let s = textureSample(atlas, samp, in.uv).rgb;
  let d = median(s) - 0.5;
  let w = fwidth(median(s));
  let a = clamp(d / max(w, 1e-4) + 0.5, 0., 1.);
  return vec4f(in.color.rgb, in.color.a * a);
}
