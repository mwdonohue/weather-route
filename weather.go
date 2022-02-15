package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
	"googlemaps.github.io/maps"
)

var EARTH_RADIUS float64 = 6371009

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

type WeatherInput struct {
	Routes        []maps.Route `json:"routes"`
	DepartureTime string       `json:"departureTime"`
}

type WeatherRetriever interface {
	Retrieve(weatherInput WeatherInput, time time.Time) (weatherCoordinates interface{}, err error)
}

type WeatherClient struct {
	OpenWeatherAPIKey string
}

func (client *WeatherClient) Retrieve(weatherInput WeatherInput, passedTime time.Time) (weatherCoordinates interface{}, err error) {
	coords, getCoordsError := getCoords(weatherInput.Routes, passedTime)
	if getCoordsError != nil {
		log.Printf("Unable to retrieve coordinates for the given route")
		return
	}
	coordsEveryFiveMiles := getCoordsEveryNMeters(coords, 8046.72)

	var weatherApiWG errgroup.Group
	weatherCoords := make([]CoordinateWeather, 0)
	for _, tempCoordTime := range coordsEveryFiveMiles {
		// Get weather data for coordinate
		coordTime := tempCoordTime
		weatherApiWG.Go(func() error {
			weatherForCoord, weatherAPIError := http.Get("https://api.openweathermap.org/data/2.5/onecall?units=imperial&lat=" +
				fmt.Sprint(coordTime.Coord.Lat) + "&lon=" +
				fmt.Sprint(coordTime.Coord.Lng) + "&exclude=current,minutely,daily,alerts&appid=" +
				client.OpenWeatherAPIKey)

			// Make sure the response returns 200
			if weatherForCoord.StatusCode != 200 {
				log.Printf("Unable to retrieve weather data for coordinate: internal API error with code %d\n", weatherForCoord.StatusCode)
				return fmt.Errorf("internal API Error: %d", weatherForCoord.StatusCode)
			}

			if weatherAPIError != nil {
				log.Printf("Unable to retrieve weather data for coordinate: %s\n", weatherAPIError.Error())
				return weatherAPIError
			}
			body, parseBodyError := ioutil.ReadAll(weatherForCoord.Body)
			if parseBodyError != nil {
				log.Printf("Unable to parse the body of the weather for coordinate: %s\n", parseBodyError.Error())
				return parseBodyError
			}
			var weather map[string]interface{}
			unmarshallingError := json.Unmarshal(body, &weather)
			if unmarshallingError != nil {
				log.Printf("Unable to unmarshall the weather's JSON data for coordinate: %s\n", unmarshallingError.Error())
				return unmarshallingError
			}

			// Find hourly entry associated with hour in coordTime
			for _, hourlyEntry := range weather["hourly"].([]interface{}) {
				unixTime := time.Unix(int64(hourlyEntry.(map[string]interface{})["dt"].(float64)), 0)
				unixTime = unixTime.UTC()
				if unixTime.Hour() == coordTime.TimeAtCoord.Hour() {
					weatherCoords = append(weatherCoords,
						CoordinateWeather{
							Coord: coordTime.Coord,
							WeatherData: Weather{
								Temperature:   hourlyEntry.(map[string]interface{})["temp"].(float64),
								Precipitation: hourlyEntry.(map[string]interface{})["pop"].(float64),
								Icon:          hourlyEntry.(map[string]interface{})["weather"].([]interface{})[0].(map[string]interface{})["icon"].(string),
							},
							Time: coordTime.TimeAtCoord,
						})
					break
				}
			}
			return nil
		})
	}
	retrievalError := weatherApiWG.Wait()
	if retrievalError != nil {
		return nil, retrievalError
	}
	return weatherCoords, nil
}

func getCoords(routes []maps.Route, currentTime time.Time) ([]CoordTime, error) {
	retVal := make([]CoordTime, 0)
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

func getCoordsEveryNMeters(path []CoordTime, distance float64) []CoordTime {
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
