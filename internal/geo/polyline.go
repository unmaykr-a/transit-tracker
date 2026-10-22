package geo

import "errors"

// DecodePolyline decodes a Google encoded polyline (precision 5) into [lon, lat] pairs,
// the GeoJSON coordinate order.
func DecodePolyline(s string) ([][2]float64, error) {
	var out [][2]float64
	var lat, lon int
	for i := 0; i < len(s); {
		var dLat, dLon int
		var err error
		if dLat, i, err = decodeValue(s, i); err != nil {
			return nil, err
		}
		if dLon, i, err = decodeValue(s, i); err != nil {
			return nil, err
		}
		lat += dLat
		lon += dLon
		out = append(out, [2]float64{float64(lon) / 1e5, float64(lat) / 1e5})
	}
	return out, nil
}

func decodeValue(s string, i int) (int, int, error) {
	result, shift := 0, 0
	for {
		if i >= len(s) {
			return 0, i, errors.New("polyline: truncated")
		}
		b := int(s[i]) - 63
		i++
		if b < 0 || b > 63 {
			return 0, i, errors.New("polyline: invalid character")
		}
		result |= (b & 0x1f) << shift
		shift += 5
		if b < 0x20 {
			break
		}
	}
	if result&1 != 0 {
		return ^(result >> 1), i, nil
	}
	return result >> 1, i, nil
}

// EncodePolyline encodes [lon, lat] pairs as a precision-5 Google polyline.
func EncodePolyline(pts [][2]float64) string {
	var buf []byte
	var pLat, pLon int
	for _, p := range pts {
		lat, lon := round5(p[1]), round5(p[0])
		buf = encodeValue(buf, lat-pLat)
		buf = encodeValue(buf, lon-pLon)
		pLat, pLon = lat, lon
	}
	return string(buf)
}

func round5(f float64) int {
	if f < 0 {
		return int(f*1e5 - 0.5)
	}
	return int(f*1e5 + 0.5)
}

func encodeValue(buf []byte, v int) []byte {
	u := v << 1
	if v < 0 {
		u = ^u
	}
	for u >= 0x20 {
		buf = append(buf, byte((0x20|(u&0x1f))+63))
		u >>= 5
	}
	return append(buf, byte(u+63))
}
