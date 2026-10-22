// Package geo has the small spherical-geometry helpers the app needs.
package geo

import "math"

const earthRadiusM = 6371008.8

func rad(d float64) float64 { return d * math.Pi / 180 }

// DistanceM is the haversine distance in metres.
func DistanceM(lat1, lon1, lat2, lon2 float64) float64 {
	dLat, dLon := rad(lat2-lat1), rad(lon2-lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusM * math.Asin(math.Min(1, math.Sqrt(a)))
}

// BearingDeg is the initial bearing from point 1 to point 2, 0..360 clockwise from north.
func BearingDeg(lat1, lon1, lat2, lon2 float64) float64 {
	φ1, φ2, Δλ := rad(lat1), rad(lat2), rad(lon2-lon1)
	y := math.Sin(Δλ) * math.Cos(φ2)
	x := math.Cos(φ1)*math.Sin(φ2) - math.Sin(φ1)*math.Cos(φ2)*math.Cos(Δλ)
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
}
