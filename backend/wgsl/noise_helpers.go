package wgsl

func (e *Emitter) emitPerlinHelpers() {
	e.buf.WriteString(`
const lfx_pi: f32 = 3.141592653589793;

fn lfx_hash11(x: f32) -> f32 {
    return fract(sin(x * 127.1) * 43758.5453123);
}

fn lfx_hash21(p: vec2<f32>) -> f32 {
    return fract(sin(dot(p, vec2<f32>(127.1, 311.7))) * 43758.5453123);
}

fn lfx_hash31(p: vec3<f32>) -> f32 {
    return fract(sin(dot(p, vec3<f32>(127.1, 311.7, 74.7))) * 43758.5453123);
}

fn lfx_fade(t: f32) -> f32 {
    return t * t * t * (t * (t * 6.0 - 15.0) + 10.0);
}

fn lfx_lerp(a: f32, b: f32, t: f32) -> f32 {
    return a + t * (b - a);
}

fn lfx_perlin_grad1(x: f32) -> f32 {
    return lfx_hash11(x) * 2.0 - 1.0;
}

fn lfx_perlin_grad2(p: vec2<f32>) -> vec2<f32> {
    let angle = 2.0 * lfx_pi * lfx_hash21(p);
    return vec2<f32>(cos(angle), sin(angle));
}

fn lfx_perlin_grad3(p: vec3<f32>) -> vec3<f32> {
    let polar = 2.0 * lfx_hash31(p) - 1.0;
    let angle = 2.0 * lfx_pi * lfx_hash31(p + vec3<f32>(19.19, 7.13, 3.17));
    let radius = sqrt(max(0.0, 1.0 - polar * polar));
    return vec3<f32>(radius * cos(angle), radius * sin(angle), polar);
}

fn lfx_perlin_base1(x: f32) -> f32 {
    let x0 = floor(x);
    let x1 = x0 + 1.0;
    let tx = x - x0;
    let n0 = lfx_perlin_grad1(x0) * tx;
    let n1 = lfx_perlin_grad1(x1) * (tx - 1.0);
    return lfx_lerp(n0, n1, lfx_fade(tx)) * 2.0;
}

fn lfx_perlin_base2(p: vec2<f32>) -> f32 {
    let cell = floor(p);
    let fracp = p - cell;
    let g00 = lfx_perlin_grad2(cell);
    let g10 = lfx_perlin_grad2(cell + vec2<f32>(1.0, 0.0));
    let g01 = lfx_perlin_grad2(cell + vec2<f32>(0.0, 1.0));
    let g11 = lfx_perlin_grad2(cell + vec2<f32>(1.0, 1.0));
    let n00 = dot(g00, fracp);
    let n10 = dot(g10, fracp - vec2<f32>(1.0, 0.0));
    let n01 = dot(g01, fracp - vec2<f32>(0.0, 1.0));
    let n11 = dot(g11, fracp - vec2<f32>(1.0, 1.0));
    let u = lfx_fade(fracp.x);
    let v = lfx_fade(fracp.y);
    return lfx_lerp(lfx_lerp(n00, n10, u), lfx_lerp(n01, n11, u), v);
}

fn lfx_perlin_base3(p: vec3<f32>) -> f32 {
    let cell = floor(p);
    let fracp = p - cell;
    let n000 = dot(lfx_perlin_grad3(cell), fracp);
    let n100 = dot(lfx_perlin_grad3(cell + vec3<f32>(1.0, 0.0, 0.0)), fracp - vec3<f32>(1.0, 0.0, 0.0));
    let n010 = dot(lfx_perlin_grad3(cell + vec3<f32>(0.0, 1.0, 0.0)), fracp - vec3<f32>(0.0, 1.0, 0.0));
    let n110 = dot(lfx_perlin_grad3(cell + vec3<f32>(1.0, 1.0, 0.0)), fracp - vec3<f32>(1.0, 1.0, 0.0));
    let n001 = dot(lfx_perlin_grad3(cell + vec3<f32>(0.0, 0.0, 1.0)), fracp - vec3<f32>(0.0, 0.0, 1.0));
    let n101 = dot(lfx_perlin_grad3(cell + vec3<f32>(1.0, 0.0, 1.0)), fracp - vec3<f32>(1.0, 0.0, 1.0));
    let n011 = dot(lfx_perlin_grad3(cell + vec3<f32>(0.0, 1.0, 1.0)), fracp - vec3<f32>(0.0, 1.0, 1.0));
    let n111 = dot(lfx_perlin_grad3(cell + vec3<f32>(1.0, 1.0, 1.0)), fracp - vec3<f32>(1.0, 1.0, 1.0));
    let u = lfx_fade(fracp.x);
    let v = lfx_fade(fracp.y);
    let w = lfx_fade(fracp.z);
    let nx00 = lfx_lerp(n000, n100, u);
    let nx10 = lfx_lerp(n010, n110, u);
    let nx01 = lfx_lerp(n001, n101, u);
    let nx11 = lfx_lerp(n011, n111, u);
    let nxy0 = lfx_lerp(nx00, nx10, v);
    let nxy1 = lfx_lerp(nx01, nx11, v);
    return lfx_lerp(nxy0, nxy1, w);
}

fn lfx_perlin1(x: f32) -> f32 {
    var total = 0.0;
    var amplitude = 1.0;
    var frequency = 1.0;
    var max_amplitude = 0.0;
    for (var octave: i32 = 0; octave < 3; octave = octave + 1) {
        total = total + amplitude * lfx_perlin_base1(x * frequency);
        max_amplitude = max_amplitude + amplitude;
        amplitude = amplitude / 2.0;
        frequency = frequency * 2.0;
    }
    return total / max_amplitude;
}

fn lfx_perlin2(p: vec2<f32>) -> f32 {
    var total = 0.0;
    var amplitude = 1.0;
    var frequency = 1.0;
    var max_amplitude = 0.0;
    for (var octave: i32 = 0; octave < 3; octave = octave + 1) {
        total = total + amplitude * lfx_perlin_base2(p * frequency);
        max_amplitude = max_amplitude + amplitude;
        amplitude = amplitude / 2.0;
        frequency = frequency * 2.0;
    }
    return total / max_amplitude;
}

fn lfx_perlin3(p: vec3<f32>) -> f32 {
    var total = 0.0;
    var amplitude = 1.0;
    var frequency = 1.0;
    var max_amplitude = 0.0;
    for (var octave: i32 = 0; octave < 3; octave = octave + 1) {
        total = total + amplitude * lfx_perlin_base3(p * frequency);
        max_amplitude = max_amplitude + amplitude;
        amplitude = amplitude / 2.0;
        frequency = frequency * 2.0;
    }
    return total / max_amplitude;
}

`)
}

func (e *Emitter) emitVoronoiHelpers() {
	e.buf.WriteString(`
fn lfx_rand2d_to1d(value: vec2<f32>, dot_dir: vec2<f32>) -> f32 {
    return fract(sin(dot(cos(value), dot_dir)) * 143758.5453);
}

fn lfx_rand2d_to2d(value: vec2<f32>) -> vec2<f32> {
    return vec2<f32>(
        lfx_rand2d_to1d(value, vec2<f32>(12.989, 78.233)),
        lfx_rand2d_to1d(value, vec2<f32>(39.346, 11.135))
    );
}

fn lfx_rand3d_to1d(value: vec3<f32>, dot_dir: vec3<f32>) -> f32 {
    return fract(sin(dot(cos(value), dot_dir)) * 143758.5453);
}

fn lfx_rand3d_to3d(value: vec3<f32>) -> vec3<f32> {
    return vec3<f32>(
        lfx_rand3d_to1d(value, vec3<f32>(12.989, 78.233, 37.719)),
        lfx_rand3d_to1d(value, vec3<f32>(39.346, 11.135, 83.155)),
        lfx_rand3d_to1d(value, vec3<f32>(73.156, 52.235, 9.151))
    );
}

fn lfx_voronoi2_data(p: vec2<f32>) -> vec3<f32> {
    let value = p / 10.0;
    let base_cell = floor(value);
    var min_dist = 10.0;
    var to_closest = vec2<f32>(0.0, 0.0);
    var closest_cell = vec2<i32>(i32(base_cell.x), i32(base_cell.y));

    for (var ox: i32 = -1; ox <= 1; ox = ox + 1) {
        for (var oy: i32 = -1; oy <= 1; oy = oy + 1) {
            let cell = base_cell + vec2<f32>(f32(ox), f32(oy));
            let to_cell = cell + lfx_rand2d_to2d(cell) - value;
            let dist = length(to_cell);
            if (dist < min_dist) {
                min_dist = dist;
                to_closest = to_cell;
                closest_cell = vec2<i32>(i32(cell.x), i32(cell.y));
            }
        }
    }

    var min_edge_distance = 10.0;
    for (var ox: i32 = -1; ox <= 1; ox = ox + 1) {
        for (var oy: i32 = -1; oy <= 1; oy = oy + 1) {
            let cell_i = vec2<i32>(i32(base_cell.x) + ox, i32(base_cell.y) + oy);
            if (all(cell_i == closest_cell)) {
                continue;
            }
            let cell = vec2<f32>(f32(cell_i.x), f32(cell_i.y));
            let to_cell = cell + lfx_rand2d_to2d(cell) - value;
            let diff = to_cell - to_closest;
            let diff_len = length(diff);
            if (diff_len == 0.0) {
                continue;
            }
            let to_center = (to_closest + to_cell) * 0.5;
            let edge_distance = dot(to_center, diff / diff_len);
            min_edge_distance = min(min_edge_distance, edge_distance);
        }
    }

    let closest = vec2<f32>(f32(closest_cell.x), f32(closest_cell.y));
    let random = lfx_rand2d_to1d(closest, vec2<f32>(12.9898, 78.233));
    return vec3<f32>(min_dist, random, min_edge_distance);
}

fn lfx_voronoi3_data(p: vec3<f32>) -> vec3<f32> {
    let value = p / 10.0;
    let base_cell = floor(value);
    var min_dist = 10.0;
    var to_closest = vec3<f32>(0.0, 0.0, 0.0);
    var closest_cell = vec3<i32>(i32(base_cell.x), i32(base_cell.y), i32(base_cell.z));

    for (var ox: i32 = -1; ox <= 1; ox = ox + 1) {
        for (var oy: i32 = -1; oy <= 1; oy = oy + 1) {
            for (var oz: i32 = -1; oz <= 1; oz = oz + 1) {
                let cell = base_cell + vec3<f32>(f32(ox), f32(oy), f32(oz));
                let to_cell = cell + lfx_rand3d_to3d(cell) - value;
                let dist = length(to_cell);
                if (dist < min_dist) {
                    min_dist = dist;
                    to_closest = to_cell;
                    closest_cell = vec3<i32>(i32(cell.x), i32(cell.y), i32(cell.z));
                }
            }
        }
    }

    var min_edge_distance = 10.0;
    for (var ox: i32 = -1; ox <= 1; ox = ox + 1) {
        for (var oy: i32 = -1; oy <= 1; oy = oy + 1) {
            for (var oz: i32 = -1; oz <= 1; oz = oz + 1) {
                let cell_i = vec3<i32>(i32(base_cell.x) + ox, i32(base_cell.y) + oy, i32(base_cell.z) + oz);
                if (all(cell_i == closest_cell)) {
                    continue;
                }
                let cell = vec3<f32>(f32(cell_i.x), f32(cell_i.y), f32(cell_i.z));
                let to_cell = cell + lfx_rand3d_to3d(cell) - value;
                let diff = to_cell - to_closest;
                let diff_len = length(diff);
                if (diff_len == 0.0) {
                    continue;
                }
                let to_center = (to_closest + to_cell) * 0.5;
                let edge_distance = dot(to_center, diff / diff_len);
                min_edge_distance = min(min_edge_distance, edge_distance);
            }
        }
    }

    let closest = vec3<f32>(f32(closest_cell.x), f32(closest_cell.y), f32(closest_cell.z));
    let random = lfx_rand3d_to1d(closest, vec3<f32>(12.9898, 78.233, 37.719));
    return vec3<f32>(min_dist, random, min_edge_distance);
}

fn lfx_voronoi2(p: vec2<f32>) -> f32 {
    return lfx_voronoi2_data(p).y;
}

fn lfx_voronoi3(p: vec3<f32>) -> f32 {
    return lfx_voronoi3_data(p).y;
}

fn lfx_voronoi_border3(p: vec3<f32>) -> f32 {
    return smoothstep(0.0, 0.08, lfx_voronoi3_data(p).z);
}

`)
}

func (e *Emitter) emitWorleyHelpers() {
	e.buf.WriteString(`
const lfx_worley_poisson_count: array<u32, 256> = array<u32, 256>(
    4u, 3u, 1u, 1u, 1u, 2u, 4u, 2u, 2u, 2u, 5u, 1u, 0u, 2u, 1u, 2u, 2u, 0u, 4u, 3u, 2u, 1u, 2u, 1u, 3u, 2u, 2u, 4u, 2u, 2u, 5u, 1u,
    2u, 3u, 2u, 2u, 2u, 2u, 2u, 3u, 2u, 4u, 2u, 5u, 3u, 2u, 2u, 2u, 5u, 3u, 3u, 5u, 2u, 1u, 3u, 3u, 4u, 4u, 2u, 3u, 0u, 4u, 2u, 2u,
    2u, 1u, 3u, 2u, 2u, 2u, 3u, 3u, 3u, 1u, 2u, 0u, 2u, 1u, 1u, 2u, 2u, 2u, 2u, 5u, 3u, 2u, 3u, 2u, 3u, 2u, 2u, 1u, 0u, 2u, 1u, 1u,
    2u, 1u, 2u, 2u, 1u, 3u, 4u, 2u, 2u, 2u, 5u, 4u, 2u, 4u, 2u, 2u, 5u, 4u, 3u, 2u, 2u, 5u, 4u, 3u, 3u, 3u, 5u, 2u, 2u, 2u, 2u, 2u,
    3u, 1u, 1u, 4u, 2u, 1u, 3u, 3u, 4u, 3u, 2u, 4u, 3u, 3u, 3u, 4u, 5u, 1u, 4u, 2u, 4u, 3u, 1u, 2u, 3u, 5u, 3u, 2u, 1u, 3u, 1u, 3u,
    3u, 3u, 2u, 3u, 1u, 5u, 5u, 4u, 2u, 2u, 4u, 1u, 3u, 4u, 1u, 5u, 3u, 3u, 5u, 3u, 4u, 3u, 2u, 2u, 1u, 1u, 1u, 1u, 1u, 2u, 4u, 5u,
    4u, 5u, 4u, 2u, 1u, 5u, 1u, 1u, 2u, 3u, 3u, 3u, 2u, 5u, 2u, 3u, 3u, 2u, 0u, 2u, 1u, 1u, 4u, 2u, 1u, 3u, 2u, 1u, 2u, 2u, 3u, 2u,
    5u, 5u, 3u, 4u, 5u, 5u, 2u, 4u, 4u, 5u, 3u, 2u, 2u, 2u, 1u, 4u, 2u, 3u, 3u, 4u, 2u, 5u, 4u, 2u, 4u, 2u, 2u, 2u, 4u, 5u, 3u, 2u
);

fn lfx_worley_seed(xi: i32, yi: i32, zi: i32, wi: i32) -> u32 {
    return 702395077u * bitcast<u32>(xi) +
        915488749u * bitcast<u32>(yi) +
        2120969693u * bitcast<u32>(zi) +
        1234567891u * bitcast<u32>(wi);
}

fn lfx_worley4(p: vec4<f32>) -> f32 {
    let base = vec4<i32>(floor(p));
    var best = 999999.9;

    for (var xi: i32 = base.x - 1; xi <= base.x + 1; xi = xi + 1) {
        for (var yi: i32 = base.y - 1; yi <= base.y + 1; yi = yi + 1) {
            for (var zi: i32 = base.z - 1; zi <= base.z + 1; zi = zi + 1) {
                for (var wi: i32 = base.w - 1; wi <= base.w + 1; wi = wi + 1) {
                    var seed = lfx_worley_seed(xi, yi, zi, wi);
                    let count = lfx_worley_poisson_count[seed >> 24u];
                    seed = 1402024253u * seed + 586950981u;

                    for (var j: u32 = 0u; j < count; j = j + 1u) {
                        seed = 1402024253u * seed + 586950981u;
                        let fx = (f32(seed) + 0.5) * (1.0 / 4294967296.0);
                        seed = 1402024253u * seed + 586950981u;
                        let fy = (f32(seed) + 0.5) * (1.0 / 4294967296.0);
                        seed = 1402024253u * seed + 586950981u;
                        let fz = (f32(seed) + 0.5) * (1.0 / 4294967296.0);
                        seed = 1402024253u * seed + 586950981u;
                        let fw = (f32(seed) + 0.5) * (1.0 / 4294967296.0);

                        let dx = f32(xi) + fx - p.x;
                        let dy = f32(yi) + fy - p.y;
                        let dz = f32(zi) + fz - p.z;
                        let dw = f32(wi) + fw - p.w;
                        let d2 = dx * dx + dy * dy + dz * dz + dw * dw;
                        best = min(best, d2);
                    }
                }
            }
        }
    }

    return sqrt(best);
}

fn lfx_worley2(p: vec2<f32>) -> f32 {
    return lfx_worley4(vec4<f32>(p.x, p.y, 0.0, 0.0));
}

fn lfx_worley3(p: vec3<f32>) -> f32 {
    return lfx_worley4(vec4<f32>(p.x, p.y, p.z, 0.0));
}

`)
}
