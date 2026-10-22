// Package eta matches vehicles to route patterns, measures how far along the
// route they are, and predicts arrival times at the stops ahead.
package eta

import "math"

// Shape is a pattern polyline prepared for projection: points in a local
// metric plane plus cumulative distance along the line.
type Shape struct {
	pts  [][2]float64 // [lon, lat]
	xy   [][2]float64 // metres in an equirectangular plane around the shape
	cum  []float64    // cumulative metres at each point
	lat0 float64
}

const mPerDegLat = 111_320.0

// NewShape prepares a [lon, lat] polyline.
func NewShape(pts [][2]float64) *Shape {
	s := &Shape{pts: pts}
	if len(pts) == 0 {
		return s
	}
	for _, p := range pts {
		s.lat0 += p[1]
	}
	s.lat0 /= float64(len(pts))
	s.xy = make([][2]float64, len(pts))
	s.cum = make([]float64, len(pts))
	for i, p := range pts {
		s.xy[i] = s.toXY(p[1], p[0])
		if i > 0 {
			s.cum[i] = s.cum[i-1] + math.Hypot(s.xy[i][0]-s.xy[i-1][0], s.xy[i][1]-s.xy[i-1][1])
		}
	}
	return s
}

func (s *Shape) toXY(lat, lon float64) [2]float64 {
	return [2]float64{lon * mPerDegLat * math.Cos(s.lat0*math.Pi/180), lat * mPerDegLat}
}

// Length is the total length in metres.
func (s *Shape) Length() float64 {
	if len(s.cum) == 0 {
		return 0
	}
	return s.cum[len(s.cum)-1]
}

// Projection is where a point falls on the shape.
type Projection struct {
	Along   float64 // metres from the start of the shape
	Offset  float64 // perpendicular distance from the shape, metres
	Bearing float64 // direction of travel at that point, degrees from north
}

// Project finds the closest point on the shape at or after minAlong metres.
func (s *Shape) Project(lat, lon, minAlong float64) (Projection, bool) {
	if len(s.xy) < 2 {
		return Projection{}, false
	}
	p := s.toXY(lat, lon)
	best := Projection{Offset: math.Inf(1)}
	for i := 1; i < len(s.xy); i++ {
		if s.cum[i] < minAlong {
			continue
		}
		a, b := s.xy[i-1], s.xy[i]
		dx, dy := b[0]-a[0], b[1]-a[1]
		seg := dx*dx + dy*dy
		t := 0.0
		if seg > 0 {
			t = ((p[0]-a[0])*dx + (p[1]-a[1])*dy) / seg
			t = math.Max(0, math.Min(1, t))
		}
		along := s.cum[i-1] + t*(s.cum[i]-s.cum[i-1])
		if along < minAlong {
			continue
		}
		off := math.Hypot(p[0]-(a[0]+t*dx), p[1]-(a[1]+t*dy))
		if off < best.Offset {
			best = Projection{Along: along, Offset: off, Bearing: math.Mod(math.Atan2(dx, dy)*180/math.Pi+360, 360)}
		}
	}
	return best, !math.IsInf(best.Offset, 1)
}

// PointAt returns the [lon, lat] position along metres from the start.
func (s *Shape) PointAt(along float64) [2]float64 {
	n := len(s.pts)
	switch {
	case n == 0:
		return [2]float64{}
	case along <= 0:
		return s.pts[0]
	case along >= s.Length():
		return s.pts[n-1]
	}
	lo, hi := 0, n-1
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if s.cum[mid] <= along {
			lo = mid
		} else {
			hi = mid
		}
	}
	span := s.cum[hi] - s.cum[lo]
	t := 0.0
	if span > 0 {
		t = (along - s.cum[lo]) / span
	}
	a, b := s.pts[lo], s.pts[hi]
	return [2]float64{a[0] + t*(b[0]-a[0]), a[1] + t*(b[1]-a[1])}
}

// StopPositions projects stops (in order) onto the shape, each no earlier than
// the previous one, so loops and out-and-back shapes keep stops in sequence.
func (s *Shape) StopPositions(stops [][2]float64) []float64 {
	out := make([]float64, len(stops))
	prev := 0.0
	for i, st := range stops {
		if pr, ok := s.Project(st[1], st[0], prev); ok {
			out[i] = pr.Along
			prev = pr.Along
		} else {
			out[i] = prev
		}
	}
	return out
}

// angleDiff is the absolute difference between two bearings, 0–180.
func angleDiff(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 360)
	if d > 180 {
		d = 360 - d
	}
	return d
}
