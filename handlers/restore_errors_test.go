package handlers

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Every restore error a user can see has a translation in every language.
func TestRestoreMessages_Translated(t *testing.T) {
	t.Parallel()
	for _, lang := range []string{"en", "fr", "es", "de"} {
		raw, err := os.ReadFile("../utils/locales/" + lang + ".yaml")
		require.NoError(t, err)
		var keys map[string]string
		require.NoError(t, yaml.Unmarshal(raw, &keys))
		for key := range restoreMessages {
			assert.NotEmpty(t, keys[key], "%s.yaml has no %s", lang, key)
		}
	}
}

// A failed restore reports its translation key and what it was about, as
// well as the English text.
func TestCompleteRestore_KeyedError(t *testing.T) {
	t.Parallel()
	svc := NewBackupService(nil, t.TempDir())
	svc.CompleteRestore(0, 0, fmt.Errorf("restore: %w", restoreFailed("backup_err_truncate", "plant")))
	got := svc.RestoreSnapshot()
	assert.Equal(t, "backup_err_truncate", got.ErrorKey)
	assert.Equal(t, "plant", got.ErrorDetail)
	assert.Equal(t, "restore: Failed to empty a table: plant", got.Error)

	svc.CompleteRestore(0, 0, fmt.Errorf("something else"))
	got = svc.RestoreSnapshot()
	assert.Equal(t, "something else", got.Error)
	assert.Empty(t, got.ErrorKey, "no key from the earlier failure")
	assert.Empty(t, got.ErrorDetail)
}
