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

	"github.com/gin-gonic/gin"
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

type DirectionsRetriever interface {
	Retrieve(routePoints RoutePoints) (routes interface{}, err error)
}

type DirectionsClient struct {
	mapClient *maps.Client
}

func (client DirectionsClient) Retrieve(routePoints RoutePoints) (routes interface{}, err error) {
	computedRoutes, _, err := client.mapClient.Directions(context.Background(), &maps.DirectionsRequest{Origin: routePoints.Origin, Destination: routePoints.Destination, Mode: maps.TravelModeDriving})
	if err != nil {
		return nil, err
	}
	return computedRoutes, nil
}

func getDirections(c *gin.Context, directionsRetriever DirectionsRetriever) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Content-Type")

	var routePoints RoutePoints
	err := c.ShouldBindJSON(&routePoints)
	if err != nil {
		log.Printf("Unable to decode directions: %s\n", err.Error())

		c.JSON(http.StatusBadRequest, gin.H{"message": "Unable to decode directions"})
		return
	}

	route, directionsErr := directionsRetriever.Retrieve(routePoints)

	if directionsErr != nil {
		log.Printf("Unable to get directions: %s\n", directionsErr.Error())

		c.JSON(http.StatusInternalServerError, gin.H{"message": "Unable to get directions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"routes": route, "travelMode": "DRIVING"})
}

func getAutoCompleteSuggestions(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Content-Type")
	var input PlaceAutocompleteInput
	err := c.ShouldBindJSON(&input)

	if err != nil {
		log.Printf("Unable to decode autocomplete suggestions: %s\n", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": "Unable to decode autocomplete suggestions"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Unable to use autocomplete client"})
		return
	}

	resp := make([]string, 0)

	for _, v := range autoCompleteResponse.Predictions {
		resp = append(resp, v.Description)
	}
	if autoCompleteResponse.Predictions != nil {
		// json.NewEncoder(rw).Encode(resp)
		c.JSON(http.StatusOK, resp)
	} else {
		c.JSON(http.StatusNoContent, []string{})
	}
}

func getWeather(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Content-Type")
	var routes []maps.Route
	err := c.ShouldBindJSON(&routes)
	if err != nil {
		log.Printf("Unable to decode routes for weather: %s\n", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": "Unable to decode routes"})
		return
	}

	coords, getCoordsError := utils.GetCoords(routes)
	if getCoordsError != nil {
		log.Printf("Unable to retrieve coordinates for the given route")
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Unabel to retrieve coordinates for the given route"})
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
				c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Unable to retrieve weather data for coordiante: internal API error"})
				return
			}

			if weatherAPIError != nil {
				log.Printf("Unable to retrieve weather data for coordinate: %s\n", weatherAPIError.Error())
				weatherApiWG.Done()
				return
			}
			weatherApiWG.Done()
			body, parseBodyError := ioutil.ReadAll(weatherForCoord.Body)
			if parseBodyError != nil {
				log.Printf("Unable to parse the body of the weather for coordinate: %s\n", parseBodyError.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Unable to parse the body of the weather for coordinate"})
				return
			}
			var weather map[string]interface{}
			unmarshallingError := json.Unmarshal(body, &weather)
			if unmarshallingError != nil {
				log.Printf("Unable to unmarshall the weather's JSON data for coordinate: %s\n", unmarshallingError.Error())
				c.JSON(http.StatusBadRequest, gin.H{"message": "Unable to marshall the weather's JSON data for coordinate"})
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
	c.JSON(http.StatusOK, weatherCoords)
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

	serv := gin.Default()
	serv.POST("/weather", getWeather)
	serv.POST("/autoCompleteSuggestions", getAutoCompleteSuggestions)

	directionsClient := &DirectionsClient{mapClient: mapClient}
	serv.POST("/directions", func(c *gin.Context) {
		getDirections(c, directionsClient)
	})

	serv.StaticFS("/", http.Dir("./static"))

	serv.Run(port)
	log.Println("Listening on port: " + port)
	log.Fatal(http.ListenAndServe(port, nil))
}
