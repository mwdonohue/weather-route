package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"weather-route/utils"

	"github.com/joho/godotenv"
	"googlemaps.github.io/maps"
)

type Configuration struct {
	OpenWeatherAPIKey       string
	GoogleMapsBackendAPIKey string
}

type PlaceAutocompleteInput struct {
	PlaceToAutoComplete string `json:"placeToAutoComplete"`
}

type RoutePoints struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
}

var config Configuration
var mapClient *maps.Client

func getDirections(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Access-Control-Allow-Origin", "*")
	rw.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	decoder := json.NewDecoder(r.Body)
	var routePoints RoutePoints
	decoder.Decode(&routePoints)
	route, _, _ := mapClient.Directions(context.Background(), &maps.DirectionsRequest{
		Origin:      routePoints.Origin,
		Destination: routePoints.Destination,
		Mode:        maps.TravelModeDriving,
	})
	json.NewEncoder(rw).Encode(map[string]interface{}{"routes": route, "travelMode": "DRIVING"})
}

func getAutoCompleteSuggestions(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Access-Control-Allow-Origin", "*")
	rw.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	decoder := json.NewDecoder(r.Body)
	var input PlaceAutocompleteInput
	decoder.Decode(&input)
	autoCompleteResponse, _ := mapClient.PlaceAutocomplete(context.Background(), &maps.PlaceAutocompleteRequest{
		Input:        input.PlaceToAutoComplete,
		StrictBounds: false,
		Types:        maps.AutocompletePlaceTypeAddress,
		Components:   map[maps.Component][]string{maps.ComponentCountry: {"us"}},
	})
	if autoCompleteResponse.Predictions != nil {
		json.NewEncoder(rw).Encode(autoCompleteResponse.Predictions)
	} else {
		json.NewEncoder(rw).Encode([]string{})
	}
}

func getWeather(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Access-Control-Allow-Origin", "*")
	rw.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	switch r.Method {
	case "POST":
		decoder := json.NewDecoder(r.Body)

		var routes []maps.Route

		err := decoder.Decode(&routes)

		if err != nil {
			panic(err)
		}

		coords := utils.GetCoords(routes)
		coordsEveryFiveMiles := utils.GetCoordsEveryNMeters(coords, 8046.72)
		weatherApiWG := sync.WaitGroup{}
		weatherCoords := make([]utils.CoordinateWeather, 0)
		for _, coordTime := range coordsEveryFiveMiles {
			// Get weather data for coordinate
			weatherApiWG.Add(1)
			go func(coordTime utils.CoordTime) {
				weatherForCoord, _ := http.Get("https://api.openweathermap.org/data/2.5/onecall?units=imperial&lat=" +
					fmt.Sprint(coordTime.Coord.Lat) + "&lon=" +
					fmt.Sprint(coordTime.Coord.Lng) + "&exclude=current,minutely,daily,alerts&appid=" +
					config.OpenWeatherAPIKey)
				weatherApiWG.Done()
				body, _ := ioutil.ReadAll(weatherForCoord.Body)
				var weather map[string]interface{}
				json.Unmarshal(body, &weather)

				// Find hourly entry associated with hour in coordTime
				for _, hourlyEntry := range weather["hourly"].([]interface{}) {
					unixTime := time.Unix(int64(hourlyEntry.(map[string]interface{})["dt"].(float64)), 0)
					unixTime = unixTime.UTC()
					if unixTime.Hour() == coordTime.TimeAtCoord.Hour() {
						weatherCoords = append(weatherCoords,
							utils.CoordinateWeather{
								Coord: coordTime.Coord,
								WeatherData: utils.Weather{
									Temperature:   hourlyEntry.(map[string]interface{})["temp"].(float64),
									Precipitation: hourlyEntry.(map[string]interface{})["pop"].(float64),
									Icon:          hourlyEntry.(map[string]interface{})["weather"].([]interface{})[0].(map[string]interface{})["icon"].(string),
								},
								Time: coordTime.TimeAtCoord,
							})
						break
					}
				}
			}(coordTime)
		}
		weatherApiWG.Wait()
		json.NewEncoder(rw).Encode(weatherCoords)
	}
}

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Printf("No environment file provided")
	}
	config = Configuration{GoogleMapsBackendAPIKey: os.Getenv("MAPS_BACKEND"), OpenWeatherAPIKey: os.Getenv("WEATHER")}
}
func main() {
	log.Println("Starting server...")

	mapClient, _ = maps.NewClient(maps.WithAPIKey(config.GoogleMapsBackendAPIKey))
	port := ":" + os.Getenv("PORT")

	if os.Getenv("PORT") == "" {
		port = ":5000"
	}
	http.HandleFunc("/weather", getWeather)
	http.HandleFunc("/autoCompleteSuggestions", getAutoCompleteSuggestions)
	http.HandleFunc("/directions", getDirections)
	serve := http.FileServer(http.Dir("./static"))
	http.Handle("/", serve)
	log.Println("Listening on port: " + port)
	log.Fatal(http.ListenAndServe(port, nil))
}
