package feeds

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"testing"
	"time"
)

// A tiny protobuf encoder for building test messages.
type pb []byte

func (p pb) varint(num int, v uint64) pb {
	p = binary.AppendUvarint(p, uint64(num)<<3)
	return binary.AppendUvarint(p, v)
}
func (p pb) bytes(num int, b []byte) pb {
	p = binary.AppendUvarint(p, uint64(num)<<3|2)
	p = binary.AppendUvarint(p, uint64(len(b)))
	return append(p, b...)
}
func (p pb) str(num int, s string) pb { return p.bytes(num, []byte(s)) }
func (p pb) f32(num int, f float32) pb {
	p = binary.AppendUvarint(p, uint64(num)<<3|5)
	return binary.LittleEndian.AppendUint32(p, math.Float32bits(f))
}

func TestParseGTFSRT(t *testing.T) {
	tripDesc := pb{}.str(1, "44_ATL-Fr-harjumaa174-A-OB-1620-4cd6234").str(2, "16:20:00").str(3, "20261023").varint(4, 0).str(5, "174_9721_1_3")
	pos := pb{}.f32(1, 59.43896).f32(2, 24.77518).f32(3, 78.3).f32(5, 7.5)
	veh := pb{}.str(1, "11").str(2, "659MSN").str(3, "659MSN")
	vp := pb{}.bytes(1, tripDesc).bytes(2, pos).varint(3, 6).varint(4, 2).varint(5, 1792762321).str(7, "11903-2").bytes(8, veh)
	delay := int64(-30) // int32 fields are sign-extended varints
	arrival := pb{}.varint(1, uint64(delay)).varint(2, 1792762348)
	stu := pb{}.varint(1, 25).bytes(2, arrival).bytes(3, pb{}.varint(2, 1792762378)).str(4, "720-1")
	tu := pb{}.bytes(1, pb{}.str(1, "t2").str(2, "15:45:00").str(5, "103_1_1_3")).bytes(2, stu)
	msg := pb{}.bytes(1, pb{}.str(1, "1.0").varint(3, 1792762326)).
		bytes(2, pb{}.str(1, "e1").bytes(4, vp)).
		bytes(2, pb{}.str(1, "e2").bytes(3, tu)).
		bytes(2, pb{}.str(1, "bad").bytes(4, []byte{0x0a, 0x05, 0x01})). // truncated: skipped
		bytes(2, pb{}.str(1, "nopos").bytes(4, pb{}.bytes(8, veh)))      // no position: dropped
	f, skipped, err := ParseGTFSRT(bytes.NewReader(msg))
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	if !f.At.Equal(time.Unix(1792762326, 0)) {
		t.Errorf("header time %v", f.At)
	}
	if len(f.Vehicles) != 1 {
		t.Fatalf("vehicles = %+v", f.Vehicles)
	}
	v := f.Vehicles[0]
	if v.ID != "659MSN" || v.Line != "174" || v.StartTime != "16:20:00" || v.StopID != "11903-2" || !v.HasSpeed ||
		math.Abs(v.Lat-59.43896) > 1e-5 || math.Abs(v.Bearing-78.3) > 1e-4 || v.At.Unix() != 1792762321 {
		t.Errorf("vehicle = %+v", v)
	}
	if len(f.Trips) != 1 || f.Trips[0].Line != "103" || len(f.Trips[0].Stops) != 1 {
		t.Fatalf("trips = %+v", f.Trips)
	}
	st := f.Trips[0].Stops[0]
	if st.StopID != "720-1" || st.Seq != 25 || st.Arrival.Unix() != 1792762348 || st.Departure.Unix() != 1792762378 || st.Delay == nil || *st.Delay != -30 {
		t.Errorf("stop time = %+v", st)
	}

	if _, _, err := ParseGTFSRT(bytes.NewReader([]byte{0x12, 0x40, 0x01})); err == nil {
		t.Error("truncated envelope: want an error")
	}
}

func TestRTLine(t *testing.T) {
	for _, c := range []struct{ route, trip, region, line string }{
		{"174_9721_1_3", "44_ATL_al_21.09-25.10.26_Orumetsa+FRK-Fr-harjumaa174-A-OB-1620-4cd6234", "harjumaa", "174"},
		{"116B_1_1_3", "x", "", "116B"},
		{"", "44_ATL_al_21.09-25.10.26_Orumetsa+FRK-Fr-harjumaa180E-A-OB-1620-4cd6234", "harjumaa", "180E"},
		{"", "Talv_KP_21.09._-_25.10.2026-Fr-raplamaaK28-A-IB-1610-3c8b775", "raplamaa", "K28"},
		{"", "94_Talv_1.10.26-Fr-laane_viru18V-A-OB-1345-8f55f27", "laane_viru", "18V"},
		{"", "KOV_KP_06.-25.10.2026_kooliaasta_2026-2027-Fr-harjumaaJ7-14:45-OB-1625-fea48ce", "harjumaa", "J7"},
		{"", "something else", "", ""},
	} {
		if r, l := RTLine(c.route, c.trip); r != c.region || l != c.line {
			t.Errorf("RTLine(%q, %q) = %q, %q; want %q, %q", c.route, c.trip, r, l, c.region, c.line)
		}
	}
}

func TestParseGTFSRTFixtures(t *testing.T) {
	b, err := os.ReadFile("testdata/ytkpohja_vehicle_positions.pb")
	if err != nil {
		t.Fatal(err)
	}
	f, skipped, err := ParseGTFSRT(bytes.NewReader(b))
	if err != nil || skipped != 0 {
		t.Fatalf("err %v, skipped %d", err, skipped)
	}
	if len(f.Vehicles) < 150 {
		t.Fatalf("only %d vehicles", len(f.Vehicles))
	}
	ids := map[string]bool{}
	for _, v := range f.Vehicles {
		if v.Line == "" || v.Region == "" || v.ID == "" || v.Lat < 57.5 || v.Lat > 59.8 || v.Lon < 21.5 || v.Lon > 28.3 {
			t.Errorf("odd vehicle %+v", v)
		}
		if ids[v.ID] {
			t.Errorf("duplicate vehicle %s", v.ID)
		}
		ids[v.ID] = true
	}
	b, _ = os.ReadFile("testdata/ytkpohja_trip_updates.pb")
	tf, skipped, err := ParseGTFSRT(bytes.NewReader(b))
	if err != nil || skipped != 0 || len(tf.Trips) < 150 {
		t.Fatalf("trip updates: err %v, skipped %d, %d trips", err, skipped, len(tf.Trips))
	}
	withTimes := 0
	for _, tu := range tf.Trips {
		if tu.StartTime == "" || tu.Line == "" || tu.Region == "" {
			t.Errorf("trip update without start or line: %+v", tu.TripID)
		}
		for _, st := range tu.Stops {
			if st.StopID != "" && !st.Arrival.IsZero() {
				withTimes++
			}
		}
	}
	if withTimes < 1000 {
		t.Errorf("only %d predicted stop times", withTimes)
	}
}
