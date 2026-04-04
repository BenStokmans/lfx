package cpu

import "math"

const (
	perlinAlpha            = 2.0
	perlinBeta             = 2.0
	perlinOctaves          = 3
	voronoiCellSize        = 10.0
	voronoiBorderThickness = 10.0
	voronoiBorderStep      = 0.08
)

var worleyPoissonCount = [256]uint32{
	4, 3, 1, 1, 1, 2, 4, 2, 2, 2, 5, 1, 0, 2, 1, 2, 2, 0, 4, 3, 2, 1, 2, 1, 3, 2, 2, 4, 2, 2, 5, 1,
	2, 3, 2, 2, 2, 2, 2, 3, 2, 4, 2, 5, 3, 2, 2, 2, 5, 3, 3, 5, 2, 1, 3, 3, 4, 4, 2, 3, 0, 4, 2, 2,
	2, 1, 3, 2, 2, 2, 3, 3, 3, 1, 2, 0, 2, 1, 1, 2, 2, 2, 2, 5, 3, 2, 3, 2, 3, 2, 2, 1, 0, 2, 1, 1,
	2, 1, 2, 2, 1, 3, 4, 2, 2, 2, 5, 4, 2, 4, 2, 2, 5, 4, 3, 2, 2, 5, 4, 3, 3, 3, 5, 2, 2, 2, 2, 2,
	3, 1, 1, 4, 2, 1, 3, 3, 4, 3, 2, 4, 3, 3, 3, 4, 5, 1, 4, 2, 4, 3, 1, 2, 3, 5, 3, 2, 1, 3, 1, 3,
	3, 3, 2, 3, 1, 5, 5, 4, 2, 2, 4, 1, 3, 4, 1, 5, 3, 3, 5, 3, 4, 3, 2, 2, 1, 1, 1, 1, 1, 2, 4, 5,
	4, 5, 4, 2, 1, 5, 1, 1, 2, 3, 3, 3, 2, 5, 2, 3, 3, 2, 0, 2, 1, 1, 4, 2, 1, 3, 2, 1, 2, 2, 3, 2,
	5, 5, 3, 4, 5, 5, 2, 4, 4, 5, 3, 2, 2, 2, 1, 4, 2, 3, 3, 4, 2, 5, 4, 2, 4, 2, 2, 2, 4, 5, 3, 2,
}

type voronoiResult struct {
	distance float64
	random   float64
	edge     float64
}

func builtinPerlin(args []float64) float64 {
	switch len(args) {
	case 1:
		return perlinFBM1(args[0])
	case 2:
		return perlinFBM2(args[0], args[1])
	case 3:
		return perlinFBM3(args[0], args[1], args[2])
	default:
		return 0
	}
}

func builtinVoronoi(args []float64) float64 {
	switch len(args) {
	case 2:
		return voronoiNoise2(args[0]/voronoiCellSize, args[1]/voronoiCellSize).random
	case 3:
		return voronoiNoise3(args[0]/voronoiCellSize, args[1]/voronoiCellSize, args[2]/voronoiCellSize).random
	default:
		return 0
	}
}

func builtinVoronoiBorder(args []float64) float64 {
	if len(args) != 3 {
		return 0
	}
	noise := voronoiNoise3(args[0]/voronoiCellSize, args[1]/voronoiCellSize, args[2]/voronoiCellSize)
	return smoothstep(0, voronoiBorderStep, noise.edge)
}

func builtinWorley(args []float64) float64 {
	if len(args) < 2 || len(args) > 4 {
		return 0
	}

	var at [4]float64
	copy(at[:], args)
	return worleyDistance(at)
}

func perlinFBM1(x float64) float64 {
	total := 0.0
	amplitude := 1.0
	frequency := 1.0
	maxAmplitude := 0.0
	for range perlinOctaves {
		total += amplitude * perlinBase1(x*frequency)
		maxAmplitude += amplitude
		amplitude /= perlinAlpha
		frequency *= perlinBeta
	}
	return total / maxAmplitude
}

func perlinFBM2(x, y float64) float64 {
	total := 0.0
	amplitude := 1.0
	frequency := 1.0
	maxAmplitude := 0.0
	for range perlinOctaves {
		total += amplitude * perlinBase2(x*frequency, y*frequency)
		maxAmplitude += amplitude
		amplitude /= perlinAlpha
		frequency *= perlinBeta
	}
	return total / maxAmplitude
}

func perlinFBM3(x, y, z float64) float64 {
	total := 0.0
	amplitude := 1.0
	frequency := 1.0
	maxAmplitude := 0.0
	for range perlinOctaves {
		total += amplitude * perlinBase3(x*frequency, y*frequency, z*frequency)
		maxAmplitude += amplitude
		amplitude /= perlinAlpha
		frequency *= perlinBeta
	}
	return total / maxAmplitude
}

func perlinBase1(x float64) float64 {
	x0 := math.Floor(x)
	x1 := x0 + 1
	tx := x - x0
	n0 := grad1(x0) * tx
	n1 := grad1(x1) * (tx - 1)
	return lerp(n0, n1, fade(tx)) * 2
}

func perlinBase2(x, y float64) float64 {
	x0 := math.Floor(x)
	y0 := math.Floor(y)
	x1 := x0 + 1
	y1 := y0 + 1

	tx := x - x0
	ty := y - y0

	g00x, g00y := grad2(x0, y0)
	g10x, g10y := grad2(x1, y0)
	g01x, g01y := grad2(x0, y1)
	g11x, g11y := grad2(x1, y1)

	n00 := g00x*tx + g00y*ty
	n10 := g10x*(tx-1) + g10y*ty
	n01 := g01x*tx + g01y*(ty-1)
	n11 := g11x*(tx-1) + g11y*(ty-1)

	u := fade(tx)
	v := fade(ty)
	return lerp(lerp(n00, n10, u), lerp(n01, n11, u), v)
}

func perlinBase3(x, y, z float64) float64 {
	x0 := math.Floor(x)
	y0 := math.Floor(y)
	z0 := math.Floor(z)
	x1 := x0 + 1
	y1 := y0 + 1
	z1 := z0 + 1

	tx := x - x0
	ty := y - y0
	tz := z - z0

	n000 := dot3(grad3(x0, y0, z0), tx, ty, tz)
	n100 := dot3(grad3(x1, y0, z0), tx-1, ty, tz)
	n010 := dot3(grad3(x0, y1, z0), tx, ty-1, tz)
	n110 := dot3(grad3(x1, y1, z0), tx-1, ty-1, tz)
	n001 := dot3(grad3(x0, y0, z1), tx, ty, tz-1)
	n101 := dot3(grad3(x1, y0, z1), tx-1, ty, tz-1)
	n011 := dot3(grad3(x0, y1, z1), tx, ty-1, tz-1)
	n111 := dot3(grad3(x1, y1, z1), tx-1, ty-1, tz-1)

	u := fade(tx)
	v := fade(ty)
	w := fade(tz)

	nx00 := lerp(n000, n100, u)
	nx10 := lerp(n010, n110, u)
	nx01 := lerp(n001, n101, u)
	nx11 := lerp(n011, n111, u)
	nxy0 := lerp(nx00, nx10, v)
	nxy1 := lerp(nx01, nx11, v)
	return lerp(nxy0, nxy1, w)
}

func voronoiNoise2(x, y float64) voronoiResult {
	baseX := math.Floor(x)
	baseY := math.Floor(y)

	minDist := voronoiBorderThickness
	toClosestX := 0.0
	toClosestY := 0.0
	closestCellX := baseX
	closestCellY := baseY

	for ox := -1; ox <= 1; ox++ {
		for oy := -1; oy <= 1; oy++ {
			cellX := baseX + float64(ox)
			cellY := baseY + float64(oy)
			jitterX, jitterY := rand2dTo2d(cellX, cellY)
			toCellX := cellX + jitterX - x
			toCellY := cellY + jitterY - y
			dist := math.Hypot(toCellX, toCellY)
			if dist < minDist {
				minDist = dist
				toClosestX = toCellX
				toClosestY = toCellY
				closestCellX = cellX
				closestCellY = cellY
			}
		}
	}

	minEdgeDistance := 10.0
	for ox := -1; ox <= 1; ox++ {
		for oy := -1; oy <= 1; oy++ {
			cellX := baseX + float64(ox)
			cellY := baseY + float64(oy)
			if approxEqual(closestCellX, cellX) && approxEqual(closestCellY, cellY) {
				continue
			}

			jitterX, jitterY := rand2dTo2d(cellX, cellY)
			toCellX := cellX + jitterX - x
			toCellY := cellY + jitterY - y
			diffX := toCellX - toClosestX
			diffY := toCellY - toClosestY
			diffLen := math.Hypot(diffX, diffY)
			if diffLen == 0 {
				continue
			}
			toCenterX := (toClosestX + toCellX) * 0.5
			toCenterY := (toClosestY + toCellY) * 0.5
			edgeDistance := (toCenterX*diffX + toCenterY*diffY) / diffLen
			minEdgeDistance = math.Min(minEdgeDistance, edgeDistance)
		}
	}

	return voronoiResult{
		distance: minDist,
		random:   rand2dTo1d(closestCellX, closestCellY, 12.9898, 78.233),
		edge:     minEdgeDistance,
	}
}

func voronoiNoise3(x, y, z float64) voronoiResult {
	baseX := math.Floor(x)
	baseY := math.Floor(y)
	baseZ := math.Floor(z)

	minDist := voronoiBorderThickness
	toClosestX := 0.0
	toClosestY := 0.0
	toClosestZ := 0.0
	closestCellX := baseX
	closestCellY := baseY
	closestCellZ := baseZ

	for ox := -1; ox <= 1; ox++ {
		for oy := -1; oy <= 1; oy++ {
			for oz := -1; oz <= 1; oz++ {
				cellX := baseX + float64(ox)
				cellY := baseY + float64(oy)
				cellZ := baseZ + float64(oz)
				jitterX, jitterY, jitterZ := rand3dTo3d(cellX, cellY, cellZ)
				toCellX := cellX + jitterX - x
				toCellY := cellY + jitterY - y
				toCellZ := cellZ + jitterZ - z
				dist := math.Sqrt(toCellX*toCellX + toCellY*toCellY + toCellZ*toCellZ)
				if dist < minDist {
					minDist = dist
					toClosestX = toCellX
					toClosestY = toCellY
					toClosestZ = toCellZ
					closestCellX = cellX
					closestCellY = cellY
					closestCellZ = cellZ
				}
			}
		}
	}

	minEdgeDistance := 10.0
	for ox := -1; ox <= 1; ox++ {
		for oy := -1; oy <= 1; oy++ {
			for oz := -1; oz <= 1; oz++ {
				cellX := baseX + float64(ox)
				cellY := baseY + float64(oy)
				cellZ := baseZ + float64(oz)
				if approxEqual(closestCellX, cellX) && approxEqual(closestCellY, cellY) && approxEqual(closestCellZ, cellZ) {
					continue
				}

				jitterX, jitterY, jitterZ := rand3dTo3d(cellX, cellY, cellZ)
				toCellX := cellX + jitterX - x
				toCellY := cellY + jitterY - y
				toCellZ := cellZ + jitterZ - z
				diffX := toCellX - toClosestX
				diffY := toCellY - toClosestY
				diffZ := toCellZ - toClosestZ
				diffLen := math.Sqrt(diffX*diffX + diffY*diffY + diffZ*diffZ)
				if diffLen == 0 {
					continue
				}
				toCenterX := (toClosestX + toCellX) * 0.5
				toCenterY := (toClosestY + toCellY) * 0.5
				toCenterZ := (toClosestZ + toCellZ) * 0.5
				edgeDistance := (toCenterX*diffX + toCenterY*diffY + toCenterZ*diffZ) / diffLen
				minEdgeDistance = math.Min(minEdgeDistance, edgeDistance)
			}
		}
	}

	return voronoiResult{
		distance: minDist,
		random:   rand3dTo1d(closestCellX, closestCellY, closestCellZ, 12.9898, 78.233, 37.719),
		edge:     minEdgeDistance,
	}
}

func worleyDistance(at [4]float64) float64 {
	intAt := [4]int32{
		int32(math.Floor(at[0])),
		int32(math.Floor(at[1])),
		int32(math.Floor(at[2])),
		int32(math.Floor(at[3])),
	}

	best := math.MaxFloat64
	for xi := intAt[0] - 1; xi <= intAt[0]+1; xi++ {
		for yi := intAt[1] - 1; yi <= intAt[1]+1; yi++ {
			for zi := intAt[2] - 1; zi <= intAt[2]+1; zi++ {
				for wi := intAt[3] - 1; wi <= intAt[3]+1; wi++ {
					addWorleySamples(xi, yi, zi, wi, at, &best)
				}
			}
		}
	}
	return math.Sqrt(best)
}

func addWorleySamples(xi, yi, zi, wi int32, at [4]float64, best *float64) {
	seed := 702395077*uint32(xi) + 915488749*uint32(yi) + 2120969693*uint32(zi) + 1234567891*uint32(wi)
	count := worleyPoissonCount[seed>>24]
	seed = 1402024253*seed + 586950981

	const invUint32 = 1.0 / 4294967296.0

	for range count {
		seed = 1402024253*seed + 586950981
		fx := (float64(seed) + 0.5) * invUint32
		seed = 1402024253*seed + 586950981
		fy := (float64(seed) + 0.5) * invUint32
		seed = 1402024253*seed + 586950981
		fz := (float64(seed) + 0.5) * invUint32
		seed = 1402024253*seed + 586950981
		fw := (float64(seed) + 0.5) * invUint32

		dx := float64(xi) + fx - at[0]
		dy := float64(yi) + fy - at[1]
		dz := float64(zi) + fz - at[2]
		dw := float64(wi) + fw - at[3]
		d2 := dx*dx + dy*dy + dz*dz + dw*dw
		if d2 < *best {
			*best = d2
		}
	}
}

func fade(t float64) float64 {
	return t * t * t * (t*(t*6-15) + 10)
}

func lerp(a, b, t float64) float64 {
	return a + t*(b-a)
}

func smoothstep(edge0, edge1, x float64) float64 {
	if edge0 == edge1 {
		if x < edge0 {
			return 0
		}
		return 1
	}
	t := clamp01((x - edge0) / (edge1 - edge0))
	return t * t * (3 - 2*t)
}

func clamp01(x float64) float64 {
	return math.Max(0, math.Min(1, x))
}

func fract(x float64) float64 {
	return x - math.Floor(x)
}

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.1
}

func hash11(x float64) float64 {
	return fract(math.Sin(x*127.1) * 43758.5453123)
}

func hash21(x, y float64) float64 {
	return fract(math.Sin(x*127.1+y*311.7) * 43758.5453123)
}

func hash31(x, y, z float64) float64 {
	return fract(math.Sin(x*127.1+y*311.7+z*74.7) * 43758.5453123)
}

func grad1(x float64) float64 {
	return hash11(x)*2 - 1
}

func grad2(x, y float64) (float64, float64) {
	angle := 2 * math.Pi * hash21(x, y)
	return math.Cos(angle), math.Sin(angle)
}

func grad3(x, y, z float64) [3]float64 {
	polar := 2*hash31(x, y, z) - 1
	angle := 2 * math.Pi * hash31(x+19.19, y+7.13, z+3.17)
	radius := math.Sqrt(math.Max(0, 1-polar*polar))
	return [3]float64{
		radius * math.Cos(angle),
		radius * math.Sin(angle),
		polar,
	}
}

func dot3(g [3]float64, x, y, z float64) float64 {
	return g[0]*x + g[1]*y + g[2]*z
}

func rand2dTo1d(x, y, dx, dy float64) float64 {
	return fract(math.Sin(math.Cos(x)*dx+math.Cos(y)*dy) * 143758.5453)
}

func rand2dTo2d(x, y float64) (float64, float64) {
	return rand2dTo1d(x, y, 12.989, 78.233), rand2dTo1d(x, y, 39.346, 11.135)
}

func rand3dTo1d(x, y, z, dx, dy, dz float64) float64 {
	return fract(math.Sin(math.Cos(x)*dx+math.Cos(y)*dy+math.Cos(z)*dz) * 143758.5453)
}

func rand3dTo3d(x, y, z float64) (float64, float64, float64) {
	return rand3dTo1d(x, y, z, 12.989, 78.233, 37.719),
		rand3dTo1d(x, y, z, 39.346, 11.135, 83.155),
		rand3dTo1d(x, y, z, 73.156, 52.235, 9.151)
}
