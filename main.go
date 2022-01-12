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

// TODO: Get rid of these globals and replace with dependency injection
var config Configuration
var mapClient *maps.Client

func getDirections(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Access-Control-Allow-Origin", "*")
	rw.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	decoder := json.NewDecoder(r.Body)
	var routePoints RoutePoints

	err := decoder.Decode(&routePoints)
	if err != nil {
		log.Printf("Unable to decode directions: %s\n", err.Error())
		http.Error(rw, "Unable to decode directions", http.StatusBadRequest)
		return
	}

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
	err := decoder.Decode(&input)
	if err != nil {
		log.Printf("Unable to decode autocomplete suggestions: %s\n", err.Error())
		http.Error(rw, "Unable to decode autocomplete suggestions", http.StatusBadRequest)
		return
	}

	autoCompleteResponse, autocompleteError := mapClient.PlaceAutocomplete(context.Background(), &maps.PlaceAutocompleteRequest{
		Input:        input.PlaceToAutoComplete,
		StrictBounds: false,
		Types:        maps.AutocompletePlaceTypeAddress,
		Components:   map[maps.Component][]string{maps.ComponentCountry: {"us"}},
	})

	if autocompleteError != nil {
		log.Printf("Unable to use autocomplete client: %s\n", autocompleteError.Error())
		http.Error(rw, "Unable to use autocomplete client", 500)
		return
	}

	resp := make([]string, 0)

	for _, v := range autoCompleteResponse.Predictions {
		resp = append(resp, v.Description)
	}
	if autoCompleteResponse.Predictions != nil {
		json.NewEncoder(rw).Encode(resp)
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
			log.Printf("Unable to decode routes for weather: %s\n", err.Error())
			http.Error(rw, "Unable to decode routes", 400)
			return
		}

		coords, getCoordsError := utils.GetCoords(routes)
		if getCoordsError != nil {
			log.Printf("Unable to retrieve coordinates for the given route")
			http.Error(rw, "Unable to retrieve coordinates for the given route: %s\n", http.StatusInternalServerError)
			return
		}
		coordsEveryFiveMiles := utils.GetCoordsEveryNMeters(coords, 8046.72)
		weatherApiWG := sync.WaitGroup{}
		weatherCoords := make([]utils.CoordinateWeather, 0)
		for _, coordTime := range coordsEveryFiveMiles {
			// Get weather data for coordinate
			weatherApiWG.Add(1)
			go func(coordTime utils.CoordTime) {
				weatherForCoord, weatherAPIError := http.Get("https://api.openweathermap.org/data/2.5/onecall?units=imperial&lat=" +
					fmt.Sprint(coordTime.Coord.Lat) + "&lon=" +
					fmt.Sprint(coordTime.Coord.Lng) + "&exclude=current,minutely,daily,alerts&appid=" +
					config.OpenWeatherAPIKey)

				// Make sure the response returns 200
				if weatherForCoord.StatusCode != 200 {
					log.Printf("Unable to retrieve weather data for coordinate: internal API error with code %d\n", weatherForCoord.StatusCode)
					http.Error(rw, "Unable to retrieve weather data for coordinate: internal API error", http.StatusServiceUnavailable)
					return
				}

				if weatherAPIError != nil {
					log.Printf("Unable to retrieve weather data for coordinate: %s\n", weatherAPIError.Error())
					http.Error(rw, "Unable to retrieve weather data for a coordinate", http.StatusInternalServerError)
					weatherApiWG.Done()
					return
				}
				weatherApiWG.Done()
				body, parseBodyError := ioutil.ReadAll(weatherForCoord.Body)
				if parseBodyError != nil {
					log.Printf("Unable to parse the body of the weather for coordinate: %s\n", parseBodyError.Error())
					http.Error(rw, "Unable to parse the body of hte weather for coordinate", http.StatusInternalServerError)
					return
				}
				var weather map[string]interface{}
				unmarshallingError := json.Unmarshal(body, &weather)
				if unmarshallingError != nil {
					log.Printf("Unable to unmarshall the weather's JSON data for coordinate: %s\n", unmarshallingError.Error())
					http.Error(rw, "Unable to unmarshall the weather's JSON data for coordinate", http.StatusBadRequest)
				}

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
	// It's okay if an environment file is not provided...
	if err != nil {
		log.Printf("No environment file provided\n")
	}
	// ...but the keys must exist one way or another
	maps_backend_key, maps_key_present := os.LookupEnv("MAPS_BACKEND")
	weather_key, weather_key_present := os.LookupEnv("WEATHER")
	if !(maps_key_present || weather_key_present) {
		log.Fatal("Maps or weather API key is not present...")
	}
	config = Configuration{GoogleMapsBackendAPIKey: maps_backend_key, OpenWeatherAPIKey: weather_key}
}
func main() {
	log.Println("Starting server...")
	var err error
	mapClient, err = maps.NewClient(maps.WithAPIKey(config.GoogleMapsBackendAPIKey))

	if err != nil {
		log.Fatalf("Unable to make map client: %s", err.Error())
	}

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
