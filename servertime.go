package main

import (
	"time"
)

type ServerTimeRetriever interface {
	Retrieve() (currTime string, err error)
}

type ServerTimeClient struct {
}

func (client *ServerTimeClient) Retrieve() (currTime string, err error) {
	return time.Now().UTC().Format(time.RFC3339), nil
}
