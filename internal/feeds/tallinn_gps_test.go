package feeds

import (
	"strings"
	"testing"
)

func TestParseGPSFixture(t *testing.T) {
	vs, skipped, err := ParseGPS(openFixture(t, "tallinn_gps.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 0 || len(vs) != 481 {
		t.Fatalf("got %d vehicles, %d skipped; want 481, 0", len(vs), skipped)
	}
	counts := map[Mode]int{}
	ids := map[string]bool{}
	for _, v := range vs {
		counts[v.Mode]++
		if ids[v.ID] {
			t.Errorf("duplicate vehicle id %q", v.ID)
		}
		ids[v.ID] = true
		if v.Lat < 59.2 || v.Lat > 59.6 || v.Lon < 24.4 || v.Lon > 25.2 {
			t.Errorf("%s outside Tallinn: %v,%v", v.ID, v.Lat, v.Lon)
		}
	}
	if counts[ModeBus] != 402 || counts[ModeTram] != 46 || counts[ModeTrolley] != 33 {
		t.Errorf("mode counts = %v", counts)
	}
	want := GPSVehicle{ID: "tram-98", Mode: ModeTram, Line: "T6", Lat: 59.43348, Lon: 24.76245, Heading: 137, LowFloor: true, Destination: "Suur-Paala"}
	if vs[0] != want {
		t.Errorf("first vehicle = %+v, want %+v", vs[0], want)
	}
}

func TestParseGPSRows(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []GPSVehicle
		skip int
	}{
		{"empty feed", "", nil, 0},
		{"blank lines and CRLF", "\r\n2,12,24635530,59412040,,151,1003,Z,4,Väike-Õismäe\r\n\r\n",
			[]GPSVehicle{{ID: "bus-1003", Mode: ModeBus, Line: "12", Lat: 59.41204, Lon: 24.63553, Heading: 151, LowFloor: true, Destination: "Väike-Õismäe"}}, 0},
		{"BOM", "\ufeff1,3,24700000,59400000,,0,7,false,1,Kadriorg",
			[]GPSVehicle{{ID: "trolley-7", Mode: ModeTrolley, Line: "3", Lat: 59.4, Lon: 24.7, Heading: 0, Destination: "Kadriorg"}}, 0},
		{"missing heading", "3,T1,24751160,59443040,,,138,false,63,Kadriorg",
			[]GPSVehicle{{ID: "tram-138", Mode: ModeTram, Line: "T1", Lat: 59.44304, Lon: 24.75116, Heading: -1, Destination: "Kadriorg"}}, 0},
		{"comma in destination", "2,5,24700000,59400000,,10,1,Z,1,A, B",
			[]GPSVehicle{{ID: "bus-1", Mode: ModeBus, Line: "5", Lat: 59.4, Lon: 24.7, Heading: 10, LowFloor: true, Destination: "A, B"}}, 0},
		{"malformed rows skipped", strings.Join([]string{
			"garbage",
			"9,1,24700000,59400000,,0,1,Z,1,X", // unknown type
			"2,1,abc,59400000,,0,1,Z,1,X",      // bad lon
			"2,1,0,0,,0,1,Z,1,X",               // no fix
			"2,,24700000,59400000,,0,1,Z,1,X",  // no line
			"2,1,24700000,59400000,,0,,Z,1,X",  // no vehicle number
		}, "\n"), nil, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, skipped, err := ParseGPS(strings.NewReader(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if skipped != tt.skip {
				t.Errorf("skipped = %d, want %d", skipped, tt.skip)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("row %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
