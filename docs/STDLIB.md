# LFX Standard Library Reference

The standard library provides reusable helper modules for common creative-coding
primitives. Import any module with:

```
import "std/module" as alias
```

Then call exported functions as `alias.function_name(args)`.

---

## std/noise

Noise functions for procedural variation. See the language reference for details.

| Function | Signature | Description |
|---|---|---|
| `perlin1` | `perlin1(x)` | 1D Perlin noise → `[0,1]` |
| `perlin2` | `perlin2(x, y)` | 2D Perlin noise → `[0,1]` |
| `perlin3` | `perlin3(x, y, z)` | 3D Perlin noise → `[0,1]` |
| `voronoi2` | `voronoi2(x, y)` | 2D Voronoi distance → `[0,1]` |
| `voronoi3` | `voronoi3(x, y, z)` | 3D Voronoi distance → `[0,1]` |
| `voronoi_border3` | `voronoi_border3(x, y, z)` | 3D Voronoi cell interior → `[0,1]` |
| `worley2` | `worley2(x, y)` | 2D Worley noise → `[0,1]` |
| `worley3` | `worley3(x, y, z)` | 3D Worley noise → `[0,1]` |
| `worley4` | `worley4(x, y, z, w)` | 4D Worley noise → `[0,1]` |

---

## std/wave

Waveform generators. All functions accept a phase `t` (f32, wraps automatically
via `fract`) and return a scalar in `[0, 1]`.

```
import "std/wave" as wave

value = wave.sine(x / width + phase)
```

| Function | Signature | Description |
|---|---|---|
| `sine` | `sine(t)` | Sinusoidal wave |
| `triangle` | `triangle(t)` | Triangle wave (linear ramp up/down) |
| `sawtooth` | `sawtooth(t)` | Linear ramp 0 → 1 |
| `ramp_down` | `ramp_down(t)` | Linear ramp 1 → 0 |
| `square` | `square(t)` | 50% duty-cycle square wave |
| `pulse` | `pulse(t, width)` | Variable duty-cycle; `width ∈ [0,1]` |
| `sine_scaled` | `sine_scaled(t, freq, phase_offset)` | Sine with explicit frequency and phase offset |

**Example:**

```
import "std/wave" as wave

function sample(width, height, x, y, index, phase, params)
  px = x / max(width, 1.0)
  brightness = wave.triangle(px * 2.0 + phase)
  return brightness
end
```

---

## std/palette

Color palette helpers. All functions return a `vec3` with components in `[0,1]`.

```
import "std/palette" as palette

output rgb
function sample(...)
  return palette.rainbow(x / width + phase)
end
```

| Function | Signature | Description |
|---|---|---|
| `cosine` | `cosine(t, ar,ag,ab, br,bg,bb, cr,cg,cb, dr,dg,db)` | Inigo Quilez cosine palette (full control) |
| `rainbow` | `rainbow(t)` | Hue sweep via three-phase cosine |
| `fire` | `fire(t)` | Black → red → orange → yellow → white |
| `grayscale` | `grayscale(t)` | Neutral gray ramp |
| `cool` | `cool(t)` | Cyan-to-magenta sweep |

**`cosine` palette formula:** `color[i] = a[i] + b[i] × cos(2π × (c[i]×t + d[i]))` per
channel. The 12 scalar parameters map to the `a`, `b`, `c`, `d` vectors in RGB order.
Output is clamped to `[0,1]`.

**Example — fire glow:**

```
import "std/palette" as palette

output rgb
function sample(width, height, x, y, index, phase, params)
  t = (y / max(height, 1.0) + phase) * 0.5
  return palette.fire(fract(t))
end
```

---

## std/ease

Easing and shaping functions. Input `t` should be in `[0, 1]`; output is `[0, 1]`.
No internal clamping is applied — pass pre-clamped values for predictable results.

```
import "std/ease" as ease

brightness = ease.smoothstep(clamp(x / width, 0.0, 1.0))
```

| Function | Signature | Description |
|---|---|---|
| `smoothstep` | `smoothstep(t)` | Cubic Hermite S-curve |
| `smootherstep` | `smootherstep(t)` | Quintic S-curve (C² continuous) |
| `ease_in` | `ease_in(t)` | Quadratic ease-in |
| `ease_out` | `ease_out(t)` | Quadratic ease-out |
| `ease_in_cubic` | `ease_in_cubic(t)` | Cubic ease-in |
| `ease_out_cubic` | `ease_out_cubic(t)` | Cubic ease-out |
| `ease_in_out` | `ease_in_out(t)` | Symmetric quadratic ease-in/out |
| `gain` | `gain(t, k)` | Schlick gain; `k=1` is linear, `k>1` pulls to extremes |
| `bias` | `bias(t, k)` | Schlick bias; `k ∈ (0,1)` — do not pass 0 or 1 |
| `bounce_out` | `bounce_out(t)` | Three-segment quadratic bounce |

---

## std/geo

Geometry helpers and signed distance functions (SDFs). SDF functions return a
**signed distance** value — negative inside the shape, positive outside. Callers
are responsible for thresholding and clamping.

```
import "std/geo" as geo

d = geo.sdf_circle(x, y, cx, cy, radius)
brightness = clamp(1.0 - abs(d) / feather, 0.0, 1.0)
```

| Function | Signature | Description |
|---|---|---|
| `sdf_circle` | `sdf_circle(px, py, cx, cy, r)` | Signed distance to circle |
| `sdf_box` | `sdf_box(px, py, cx, cy, hw, hh)` | Signed distance to axis-aligned box (half-extents) |
| `sdf_line` | `sdf_line(px, py, ax, ay, bx, by)` | Distance to line segment |
| `sdf_ring` | `sdf_ring(px, py, cx, cy, r, thickness)` | Signed distance to ring/annulus |
| `polar_r` | `polar_r(px, py, cx, cy)` | Radial distance from center |
| `polar_theta` | `polar_theta(px, py, cx, cy)` | Cos(θ) from center, remapped to `[0,1]` |
| `wrap` | `wrap(v, lo, hi)` | Wrap value into `[lo, hi)`. Requires `hi > lo`. |
| `remap` | `remap(v, in_lo, in_hi, out_lo, out_hi)` | Linear remap (unclamped) |

**Note on `polar_theta`:** Returns `0.5 + 0.5 * cos(θ)` — it is a cos-proxy, not
a full angle. Left = 0, right = 1, with top/bottom at 0.5. For full angular access
use `polar_r` and two `sdf_line` calls.

---

## std/warp

Domain warping using the `__perlin` builtin directly. All outputs are in `[0,1]`.

The `fbm` functions (fractional Brownian motion) are offered in fixed octave counts
because the language does not support loops. More octaves = finer detail but more
computation.

```
import "std/warp" as warp

value = warp.warp2(x / width * 3.0, y / height * 3.0, 1.5)
```

| Function | Signature | Description |
|---|---|---|
| `fbm2_2` | `fbm2_2(x, y)` | 2D fBm, 2 octaves |
| `fbm2_4` | `fbm2_4(x, y)` | 2D fBm, 4 octaves |
| `fbm2_6` | `fbm2_6(x, y)` | 2D fBm, 6 octaves |
| `fbm2_8` | `fbm2_8(x, y)` | 2D fBm, 8 octaves |
| `fbm3_4` | `fbm3_4(x, y, z)` | 3D fBm, 4 octaves |
| `fbm3_6` | `fbm3_6(x, y, z)` | 3D fBm, 6 octaves |
| `warp2` | `warp2(x, y, strength)` | Domain-warped 2D fBm (Inigo Quilez style) |
| `turbulence2` | `turbulence2(x, y)` | Absolute-value fBm for turbulent look |

**fBm accumulation:** The `__perlin` builtin returns values centred around 0
(approximately `[-0.4, 0.4]`). Octave terms are accumulated as `perlin × amplitude`
and the running sum is mapped to `[0,1]` via `clamp(0.5 + sum, 0, 1)`.

**`warp2` offsets:** Uses the classic IQ domain warp constants (1.7, 9.2, 8.3, 2.8)
to decorrelate the two displacement samples from each other and from the final sample.

---

## std/patterns

Tiling pattern generators. All functions take UV coordinates `u, v` (typically
normalised to `[0,1]` from pixel coords) and a `scale` parameter, returning a scalar
in `[0, 1]`.

```
import "std/patterns" as patterns

u = x / max(width, 1.0)
v = y / max(height, 1.0)
brightness = patterns.checker(u, v, 8.0)
```

| Function | Signature | Description |
|---|---|---|
| `checker` | `checker(u, v, scale)` | Checkerboard; returns 0 or 1 |
| `stripes_h` | `stripes_h(u, scale)` | Horizontal stripes; returns 0 or 1 |
| `stripes_v` | `stripes_v(v, scale)` | Vertical stripes; returns 0 or 1 |
| `dots` | `dots(u, v, scale, radius)` | Circular dots; `radius ∈ (0, 0.5]` |
| `grid` | `grid(u, v, scale, line_width)` | Grid lines; 1 on lines, 0 in interior |
| `hex` | `hex(u, v, scale)` | Hexagonal tiling (skewed-grid approximation) |
| `brick` | `brick(u, v, scale, offset)` | Brick rows with alternating row offset |

**`dots` radius:** Controls dot size as a fraction of cell half-width. At `radius=0.5`
dots are tangent; below that they have gaps. Passing 0 gives all black.

**`hex` approximation:** Uses a skewed grid, not perfect hex geometry. Sufficient for
lighting effects; a corrected version would require `atan2` which is not a builtin.
