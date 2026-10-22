package feeds

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func zipDir(t *testing.T, dir string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		w, _ := zw.Create(e.Name())
		_, _ = w.Write(b)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadGTFSZip(t *testing.T) {
	g, err := ReadGTFSZip(zipDir(t, "testdata/elron_gtfs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Routes) != 2 || len(g.Trips) != 8 || len(g.StopTimes) != 8 || len(g.Stops) != 34 || len(g.Shapes) == 0 {
		t.Fatalf("routes=%d trips=%d stoptimes=%d stops=%d shapes=%d", len(g.Routes), len(g.Trips), len(g.StopTimes), len(g.Stops), len(g.Shapes))
	}
	tallinn, ok := g.Stops["10137"]
	if !ok || tallinn.Name != "Tallinn" || tallinn.Code != "21106-1" {
		t.Errorf("Tallinn station = %+v (ok=%v)", tallinn, ok)
	}
	for _, trip := range g.Trips {
		seq := g.StopTimes[trip.ID]
		if len(seq) < 2 {
			t.Errorf("trip %s has %d stops", trip.ID, len(seq))
		}
		if shape := g.Shapes[trip.ShapeID]; len(shape) < 2 {
			t.Errorf("trip %s shape %s has %d points", trip.ID, trip.ShapeID, len(shape))
		}
	}
	if _, err := ReadGTFSZip([]byte("not a zip")); err == nil {
		t.Error("garbage must fail")
	}
}
