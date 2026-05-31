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

fn circleSDF(p: vec2f, r: f32) -> f32 { return length(p) - r; }

fn ticks(p: vec2f, r0: f32, r1: f32, a0: f32, ext: f32, everyRad: f32, halfWidth: f32) -> f32 {
  let radius = length(p);
  if (radius < r0 || radius > r1) { return 0.; }
  var ang = atan2(p.y, p.x); if (ang < 0.) { ang = ang + 2.*PI; }
  var rel = ang - (a0 % (2.*PI)); if (rel < 0.) { rel = rel + 2.*PI; }
  if (rel > ext) { return 0.; }
  let m = rel % everyRad;
  let d = min(m, everyRad - m);
  return 1.0 - smoothstep(halfWidth*0.5, halfWidth, d);
}

fn segSDF(p: vec2f, a: vec2f, b: vec2f, r: f32) -> f32 {
  let pa = p - a; let ba = b - a;
  let h = clamp(dot(pa,ba)/dot(ba,ba), 0., 1.);
  return length(pa - ba*h) - r;
}

fn roundRect(p: vec2f, half: vec2f, r: f32) -> f32 {
  let q = abs(p) - half + vec2f(r);
  return min(max(q.x,q.y),0.) + length(max(q,vec2f(0.))) - r;
}

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

  // speed dial face
  let dial = circleSDF(p, 0.40);
  col = mix(col, vec3f(0.13,0.15,0.20), 1.0 - smoothstep(0., aa, dial));
  // minor + major ticks (13.5° / 54°)
  let minorT = ticks(p, 0.34, 0.38, u.sweepStart, u.sweepExtent, 13.5*PI/180., 0.6*PI/180.);
  let majorT = ticks(p, 0.32, 0.38, u.sweepStart, u.sweepExtent, 54.0*PI/180., 1.2*PI/180.);
  col = mix(col, u.tick.rgb, max(minorT, majorT) * 0.9);
  // needle
  let tip = vec2f(cos(u.speedAngle), sin(u.speedAngle)) * 0.34;
  let needle = segSDF(p, vec2f(0.,0.), tip, 0.006);
  col = mix(col, u.ringRed.rgb, 1.0 - smoothstep(0., aa, needle));
  // hub
  col = mix(col, vec3f(0.23,0.26,0.30), 1.0 - smoothstep(0., aa, circleSDF(p, 0.02)));

  // rail (right of the gauge), in canvas-normalized coords
  let q = (frag.xy - vec2f(0.78, 0.0)*res) / mn;
  let qy = frag.y / mn;
  // gear tile at cy 0.22
  let gear = roundRect(vec2f(q.x, qy - 0.22*res.y/mn), vec2f(0.08,0.08), 0.03);
  col = mix(col, vec3f(0.10,0.13,0.18), 1.0 - smoothstep(0., aa, gear));
  // throttle/brake bars
  let barH = 0.13; let cyBars = 0.5*res.y/mn;
  let inThr = roundRect(vec2f(q.x-(-0.025), qy-cyBars), vec2f(0.015, barH), 0.012);
  let thrFillTop = cyBars + barH - u.throttle*2.0*barH;
  let thrLit = step(thrFillTop, qy) * (1.0 - smoothstep(0., aa, inThr));
  col = mix(col, vec3f(0.09,0.10,0.13), 1.0 - smoothstep(0., aa, inThr));
  col = mix(col, vec3f(0.21,0.82,0.48), thrLit);
  let inBrk = roundRect(vec2f(q.x-0.025, qy-cyBars), vec2f(0.015, barH), 0.012);
  let brkFillTop = cyBars + barH - u.brake*2.0*barH;
  let brkLit = step(brkFillTop, qy) * (1.0 - smoothstep(0., aa, inBrk));
  col = mix(col, vec3f(0.09,0.10,0.13), 1.0 - smoothstep(0., aa, inBrk));
  col = mix(col, vec3f(1.0,0.35,0.30), brkLit);
  // g-circle at cy 0.8 with dot
  let gcY = 0.8*res.y/mn;
  let gc = abs(circleSDF(vec2f(q.x, qy-gcY), 0.10)) - 0.002;
  col = mix(col, vec3f(0.30,0.33,0.40), 1.0 - smoothstep(0., aa, gc));
  let gdot = circleSDF(vec2f(q.x - u.gx*0.085, qy - gcY - u.gy*0.085), 0.012);
  col = mix(col, vec3f(0.61,0.55,1.0), 1.0 - smoothstep(0., aa, gdot));

  return vec4f(col, 1.0);
}
