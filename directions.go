package main

import (
	"context"

	"googlemaps.github.io/maps"
)

type RoutePoints struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
}

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
