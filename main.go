package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"googlemaps.github.io/maps"
)

type Configuration struct {
	OpenWeatherAPIKey       string
	GoogleMapsBackendAPIKey string
}

var config Configuration

func GetDirections(c *gin.Context, directionsRetriever DirectionsRetriever) {
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

func GetAutoCompleteSuggestions(c *gin.Context, autocompleteRetriever AutoCompleteRetriever) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Content-Type")
	var input PlaceAutocompleteInput
	err := c.ShouldBindJSON(&input)

	if err != nil {
		log.Printf("Unable to decode autocomplete suggestions: %s\n", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": "Unable to decode autocomplete suggestions"})
		return
	}
	autoCompleteResponse, autocompleteError := autocompleteRetriever.Retrieve(input)
	if autocompleteError != nil {
		log.Printf("Unable to use autocomplete client: %s\n", autocompleteError.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Unable to use autocomplete client"})
		return
	}
	if autoCompleteResponse != nil {
		c.JSON(http.StatusOK, autoCompleteResponse)
	} else {
		c.JSON(http.StatusNoContent, []string{})
	}
}

func GetWeather(c *gin.Context, weatherRetriever WeatherRetriever) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Content-Type")
	var routes []maps.Route
	err := c.ShouldBindJSON(&routes)
	if err != nil {
		log.Printf("Unable to decode routes for weather: %s\n", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": "Unable to decode routes"})
		return
	}

	weatherCoords, err := weatherRetriever.Retrieve(routes)

	if err != nil {
		log.Printf("Unable to retrieve weather information: %s\n", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Unable to retrieve weather information"})
		return
	}

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
	mapClient, err := maps.NewClient(maps.WithAPIKey(config.GoogleMapsBackendAPIKey))

	if err != nil {
		log.Fatalf("Unable to make map client: %s", err.Error())
	}

	port := ":" + os.Getenv("PORT")

	if os.Getenv("PORT") == "" {
		port = ":5000"
	}

	serv := gin.Default()

	weatherClient := &WeatherClient{OpenWeatherAPIKey: config.OpenWeatherAPIKey}
	serv.POST("/weather", func(c *gin.Context) {
		GetWeather(c, weatherClient)
	})

	autocompleteClient := &AutoCompleteClient{mapClient: mapClient}
	serv.POST("/autoCompleteSuggestions", func(c *gin.Context) {
		GetAutoCompleteSuggestions(c, autocompleteClient)
	})

	directionsClient := &DirectionsClient{mapClient: mapClient}
	serv.POST("/directions", func(c *gin.Context) {
		GetDirections(c, directionsClient)
	})

	serv.StaticFS("/", http.Dir("./static"))

	serv.Run(port)
	log.Println("Listening on port: " + port)
	log.Fatal(http.ListenAndServe(port, nil))
}
