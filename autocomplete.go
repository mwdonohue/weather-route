package main

import (
	"context"

	"googlemaps.github.io/maps"
)

type PlaceAutocompleteInput struct {
	PlaceToAutoComplete string `json:"placeToAutoComplete"`
}

type AutoCompleteRetriever interface {
	Retrieve(autocompleteInput PlaceAutocompleteInput) (response interface{}, err error)
}

type AutoCompleteClient struct {
	mapClient *maps.Client
}

func (client AutoCompleteClient) Retrieve(autocompleteInput PlaceAutocompleteInput) (response interface{}, err error) {
	autoCompleteResponse, autoCompleteError := client.mapClient.PlaceAutocomplete(context.Background(), &maps.PlaceAutocompleteRequest{
		Input:        autocompleteInput.PlaceToAutoComplete,
		StrictBounds: false,
		Types:        maps.AutocompletePlaceTypeAddress,
		Components:   map[maps.Component][]string{maps.ComponentCountry: {"us"}},
	})
	if autoCompleteError != nil {
		return nil, err
	}
	resp := make([]string, 0)
	for _, v := range autoCompleteResponse.Predictions {
		resp = append(resp, v.Description)
	}
	return resp, nil
}
