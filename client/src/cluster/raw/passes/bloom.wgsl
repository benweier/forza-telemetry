@group(0) @binding(0) var samp  : sampler;
@group(0) @binding(1) var tex0  : texture_2d<f32>;   // bright/blur: source. composite: scene
@group(0) @binding(2) var tex1  : texture_2d<f32>;   // composite: bloom (bind sceneTex as a dummy for bright/blur)
struct BP { texel: vec2f, dir: vec2f, threshold: f32, intensity: f32, pad: vec2f };
@group(0) @binding(3) var<uniform> b : BP;

@vertex fn vs(@builtin(vertex_index) i : u32) -> @builtin(position) vec4f {
  var p = array<vec2f,3>(vec2f(-1.,-1.), vec2f(3.,-1.), vec2f(-1.,3.));
  return vec4f(p[i], 0., 1.);
}
fn uvOf(fc: vec4f) -> vec2f { return fc.xy * b.texel; }

@fragment fn bright(@builtin(position) fc: vec4f) -> @location(0) vec4f {
  let c = textureSample(tex0, samp, uvOf(fc)).rgb;
  let l = max(c.r, max(c.g, c.b));
  let k = max(0., l - b.threshold) / max(1e-3, 1.0 - b.threshold);
  return vec4f(c * k, 1.);
}
@fragment fn blur(@builtin(position) fc: vec4f) -> @location(0) vec4f {
  let w = array<f32,5>(0.227, 0.194, 0.121, 0.054, 0.016);
  let uv = uvOf(fc);
  var sum = textureSample(tex0, samp, uv).rgb * w[0];
  for (var i = 1; i < 5; i++) {
    let o = b.dir * f32(i) * b.texel;
    sum += textureSample(tex0, samp, uv + o).rgb * w[i];
    sum += textureSample(tex0, samp, uv - o).rgb * w[i];
  }
  return vec4f(sum, 1.);
}
@fragment fn composite(@builtin(position) fc: vec4f) -> @location(0) vec4f {
  let uv = uvOf(fc);
  let scene = textureSample(tex0, samp, uv).rgb;
  let bloom = textureSample(tex1, samp, uv).rgb * b.intensity;
  return vec4f(scene + bloom, 1.);
}
