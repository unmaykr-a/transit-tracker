package eta

import (
	"math"
	"testing"
)

// An L-shaped route: ~1 km north, then ~1 km east.
func lShape() *Shape {
	return NewShape([][2]float64{{24.70, 59.40}, {24.70, 59.409}, {24.7176, 59.409}})
}

func TestProjectAndPointAt(t *testing.T) {
	s := lShape()
	if l := s.Length(); math.Abs(l-2002) > 25 {
		t.Fatalf("length = %.0f", l)
	}
	// 20 m west of the northbound leg, halfway up.
	pr, ok := s.Project(59.4045, 24.69965, 0)
	if !ok || math.Abs(pr.Along-501) > 10 || math.Abs(pr.Offset-20) > 3 || pr.Bearing > 1 && pr.Bearing < 359 {
		t.Fatalf("projection = %+v", pr)
	}
	// On the eastbound leg the bearing is east.
	pr, _ = s.Project(59.409, 24.71, 0)
	if math.Abs(pr.Bearing-90) > 1 || pr.Along < 1000 {
		t.Fatalf("east leg = %+v", pr)
	}
	// minAlong skips earlier parts of the shape.
	pr, _ = s.Project(59.40, 24.70, 900)
	if pr.Along < 900 {
		t.Fatalf("minAlong ignored: %+v", pr)
	}
	p := s.PointAt(1500)
	if math.Abs(p[1]-59.409) > 1e-6 || p[0] < 24.70 || p[0] > 24.7176 {
		t.Fatalf("PointAt(1500) = %v", p)
	}
	if s.PointAt(-5) != s.pts[0] || s.PointAt(1e9) != s.pts[2] {
		t.Error("PointAt clamps")
	}
}

func TestStopPositionsStayOrdered(t *testing.T) {
	// Out and back along the same street: the return stop must land on the way back.
	s := NewShape([][2]float64{{24.70, 59.40}, {24.70, 59.41}, {24.70001, 59.40}})
	pos := s.StopPositions([][2]float64{{24.70, 59.40}, {24.70, 59.41}, {24.70, 59.405}})
	if !(pos[0] < pos[1] && pos[1] < pos[2]) || pos[2] < 1500 {
		t.Fatalf("positions = %v", pos)
	}
}

func TestAngleDiff(t *testing.T) {
	for _, tt := range [][3]float64{{10, 350, 20}, {0, 180, 180}, {90, 90, 0}, {359, 1, 2}} {
		if got := angleDiff(tt[0], tt[1]); math.Abs(got-tt[2]) > 1e-9 {
			t.Errorf("angleDiff(%v,%v) = %v", tt[0], tt[1], got)
		}
	}
}
