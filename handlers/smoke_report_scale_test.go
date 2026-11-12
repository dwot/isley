package handlers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
)

func TestSmokeRatingScale_CoversTheDocumentedBounds(t *testing.T) {
	t.Parallel()

	scale := handlers.SmokeRatingScale()

	require.Len(t, scale, 5, "the 1-5 scale is what both modals render")
	// Ascending and gapless: the templates emit one star per entry in order.
	for i, v := range scale {
		assert.Equal(t, i+1, v, "scale[%d] should be %d", i, i+1)
	}
}
