package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFail(t *testing.T) {
	assert := assert.New(t)

	assert.Equal(0, 0)
}
