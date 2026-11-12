package app

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

func TestAssetURL_FollowsContent(t *testing.T) {
	t.Parallel()
	v1 := assetURL(fstest.MapFS{"web/static/js/a.js": {Data: []byte("one")}})
	v2 := assetURL(fstest.MapFS{"web/static/js/a.js": {Data: []byte("two")}})

	a := v1("/static/js/a.js")
	assert.Regexp(t, `^/static/js/a\.js\?v=[0-9a-f]{8}$`, a)
	assert.Equal(t, a, v1("/static/js/a.js"), "stable")
	assert.NotEqual(t, a, v2("/static/js/a.js"), "changes with the content")
	assert.Equal(t, "/static/js/missing.js", v1("/static/js/missing.js"), "a missing file keeps its plain path")
}
