package utils

import (
	"googlemaps.github.io/maps"
	"math"
)

var EARTH_RADIUS float64 = 6371009

// Distance between two coords in meters
func computeDistanceBetween(from maps.LatLng, to maps.LatLng) float64 {
	return computeAngleBetween(from, to) * EARTH_RADIUS
}

func distanceRadians(lat1, lng1, lat2, lng2 float64) float64 {
	return arcHav(havDistance(lat1, lat2, lng1-lng2))
}

func computeAngleBetween(from, to maps.LatLng) float64 {
	return distanceRadians(degToRad(from.Lat), degToRad(from.Lng), degToRad(to.Lat), degToRad(to.Lng))
}
func computeHeading(from maps.LatLng, to maps.LatLng) float64 {
	// Convert "from" LatLng and "to" LatLng to radians
	fromLatRad := degToRad(from.Lat)
	fromLongRad := degToRad(from.Lng)
	toLatRad := degToRad(to.Lat)
	toLongRad := degToRad(to.Lng)
	distanceLng := toLongRad - fromLongRad

	retVal := math.Atan2(math.Sin(distanceLng)*math.Cos(toLatRad), math.Cos(fromLatRad)*math.Sin(toLatRad)-math.Sin(fromLatRad)*math.Cos(toLatRad)*math.Cos(distanceLng))
	return wrap(radToDeg(retVal), -180, 180)
}

func computeOffsetOrigin(to CoordTime, distance float64, heading float64) CoordTime {
	heading = degToRad(heading)

	distance /= EARTH_RADIUS

	n1 := math.Cos(distance)
	n2 := math.Sin(distance) * math.Cos(heading)
	n3 := math.Sin(distance) * math.Sin(heading)
	n4 := math.Sin(degToRad(to.Coord.Lat))

	n12 := n1 * n1
	discriminant := n2*n2*n12 + n12*n12 - n12*n4*n4

	if discriminant < 0 {
		return CoordTime{}
	}

	b := n2*n4 + math.Sqrt(discriminant)

	b /= n1*n1 + n2*n2

	a := (n4 - n2*b) / n1

	fromLatRads := math.Atan2(a, b)

	if fromLatRads < -math.Pi/2 || fromLatRads > math.Pi/2 {
		b = n2*n4 - math.Sqrt(discriminant)
		b /= n1*n1 + n2*n2
		fromLatRads = math.Atan2(a, b)
	}
	if fromLatRads < -math.Pi/2 || fromLatRads > math.Pi/2 {
		return CoordTime{}
	}

	fromLngRads := degToRad(to.Coord.Lng) - math.Atan2(n3, n1*math.Cos(fromLatRads)-n2*math.Sin(fromLatRads))
	return CoordTime{
		maps.LatLng{
			Lat: radToDeg(fromLatRads),
			Lng: radToDeg(fromLngRads),
		}, to.TimeAtCoord,
	}
}

func degToRad(deg float64) float64 {
	return deg * (math.Pi / 180)
}

func radToDeg(rad float64) float64 {
	return rad * (180 / math.Pi)
}

func wrap(n float64, min float64, max float64) float64 {
	if n >= min && n < max {
		return n
	} else {
		return mod(n-min, max-min) + min
	}
}

func mod(x float64, m float64) float64 {
	return math.Mod((math.Mod(x, m) + m), m)
}

func arcHav(x float64) float64 {
	return 2 * math.Asin(math.Sqrt(x))
}

func havDistance(lat1 float64, lat2 float64, dLng float64) float64 {
	return hav(lat1-lat2) + hav(dLng)*math.Cos(lat1)*math.Cos(lat2)
}

func hav(x float64) float64 {
	return math.Sin(x*0.5) * math.Sin(x*0.5)
}
