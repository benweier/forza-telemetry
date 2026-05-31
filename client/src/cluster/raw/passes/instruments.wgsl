struct Uniforms {
  resolution : vec2f,
  sweepStart : f32,
  sweepExtent: f32,
  rpmFrac    : f32,
  redline    : f32,
  speedAngle : f32,
  throttle   : f32,
  brake      : f32,
  gx         : f32,
  gy         : f32,
  _pad       : f32,
  ringLow    : vec4f,
  ringMid    : vec4f,
  ringRed    : vec4f,
  panel      : vec4f,
  tick       : vec4f,
};
@group(0) @binding(0) var<uniform> u : Uniforms;

@vertex
fn vs(@builtin(vertex_index) i : u32) -> @builtin(position) vec4f {
  var p = array<vec2f,3>(vec2f(-1.,-1.), vec2f(3.,-1.), vec2f(-1.,3.));
  return vec4f(p[i], 0., 1.);
}

const PI = 3.14159265;

fn arc(p: vec2f, r0: f32, r1: f32, a0: f32, a1: f32) -> f32 {
  let radius = length(p);
  var ang = atan2(p.y, p.x);
  if (ang < 0.) { ang = ang + 2.*PI; }
  let band = max(r0 - radius, radius - r1);
  let aa = a0 % (2.*PI);
  let span = a1 - a0;
  var rel = ang - aa; if (rel < 0.) { rel = rel + 2.*PI; }
  let outside = max(-rel, rel - span);
  return max(band, outside * radius);
}

@fragment
fn fs(@builtin(position) frag : vec4f) -> @location(0) vec4f {
  let res = u.resolution;
  let mn = min(res.x, res.y);
  let center = vec2f(0.32, 0.5) * res;
  let p = (frag.xy - center) / mn;
  var col = u.panel.rgb;

  let r0 = 0.42; let r1 = 0.46;
  let dBand = arc(p, r0, r1, u.sweepStart, u.sweepStart + u.sweepExtent);
  var ang = atan2(p.y, p.x); if (ang < 0.) { ang = ang + 2.*PI; }
  var rel = ang - (u.sweepStart % (2.*PI)); if (rel < 0.) { rel = rel + 2.*PI; }
  let frac = rel / u.sweepExtent;
  let filled = step(frac, u.rpmFrac);
  var ramp = mix(u.ringLow.rgb, u.ringMid.rgb, smoothstep(0.0, 0.7, frac));
  ramp = mix(ramp, u.ringRed.rgb, smoothstep(0.82, 1.0, frac));
  let aa = 1.5 / mn;
  let band = 1.0 - smoothstep(0.0, aa, dBand);
  let trackCol = vec3f(0.08,0.10,0.13);
  col = mix(col, mix(trackCol, ramp, filled), band);
  return vec4f(col, 1.0);
}
