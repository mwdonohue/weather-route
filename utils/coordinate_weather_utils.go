package utils

import (
	"log"
	"time"

	"googlemaps.github.io/maps"
)

type CoordTime struct {
	Coord       maps.LatLng `json:"coord"`
	TimeAtCoord time.Time   `json:"timeAtCoord"`
}

type CoordinateWeather struct {
	Coord       maps.LatLng `json:"coordinate"`
	WeatherData Weather     `json:"weatherData"`
	Time        time.Time   `json:"time"`
}

type Weather struct {
	Temperature   float64 `json:"temperature"`
	Precipitation float64 `json:"precipChance"`
	Icon          string  `json:"weatherIcon"`
}

func GetCoords(routes []maps.Route) ([]CoordTime, error) {
	retVal := make([]CoordTime, 0)

	currentTime := time.Now().UTC()
	if len(routes) > 0 {
		rte := routes[0]

		for i := 0; i < len(rte.Legs); i++ {
			leg := rte.Legs[i]
			for j := 0; j < len(leg.Steps); j++ {
				step := leg.Steps[j]
				if len(step.Steps) > 0 {
					for k := 0; k < len(step.Steps); k++ {
						currentStep := step.Steps[k]
						polyLine := currentStep.Polyline
						coords, decodeError := polyLine.Decode()
						if decodeError != nil {
							log.Printf("Unable to decode polyline: %s\n", decodeError.Error())
							return nil, decodeError
						}

						for _, coord := range coords {
							retVal = append(retVal, CoordTime{Coord: coord, TimeAtCoord: currentTime})
						}
						currentTime = currentTime.Add(currentStep.Duration)
					}
				} else {
					polyLine := step.Polyline
					coords, decodeError := polyLine.Decode()
					if decodeError != nil {
						log.Printf("Unable to decode polyline: %s\n", decodeError.Error())
						return nil, decodeError
					}
					for _, coord := range coords {
						retVal = append(retVal, CoordTime{Coord: coord, TimeAtCoord: currentTime})
					}
					currentTime = currentTime.Add(step.Duration)
				}
			}
		}
	}
	return retVal, nil
}

func GetCoordsEveryNMeters(path []CoordTime, distance float64) []CoordTime {
	retVal := make([]CoordTime, 0)

	p0 := path[0]
	retVal = append(retVal, p0)

	if len(path) > 2 {
		tmp := float64(0)
		prev := p0

		for _, p := range path {
			tmp += computeDistanceBetween(prev.Coord, p.Coord)
			if tmp < distance {
				prev = p
				continue
			} else {
				diff := tmp - distance
				heading := computeHeading(prev.Coord, p.Coord)

				pp := computeOffsetOrigin(p, diff, heading)

				tmp = 0
				prev = pp
				retVal = append(retVal, pp)
				continue
			}
		}
		lastPoint := path[len(path)-1]
		retVal = append(retVal, lastPoint)
	}
	return retVal
}
