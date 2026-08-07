package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"isley/logger"
	"isley/utils"

	"github.com/gin-gonic/gin"
)

// apiKeyPrefixLen is how many leading characters of a generated key are kept in
// the prefix column. The prefix lets a key be identified in the UI and lets
// auth narrow the bcrypt comparison to the matching candidate row instead of
// hashing against every key. A generated key is 32 hex chars, so 24 chars (96
// bits) of entropy remain hidden — far beyond brute-force from the prefix.
const apiKeyPrefixLen = 8

// APIKeyInfo is the secret-free view of an API key returned to the settings
// page. The hash is never exposed; the prefix is shown so a key can be matched
// to the device that holds it.
type APIKeyInfo struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Prefix   string `json:"prefix"`
	LastUsed string `json:"last_used"`
	Created  string `json:"created"`
}

// apiKeyPrefix returns the leading characters used to identify a key.
func apiKeyPrefix(plaintext string) string {
	if len(plaintext) < apiKeyPrefixLen {
		return plaintext
	}
	return plaintext[:apiKeyPrefixLen]
}

// ListAPIKeys returns every stored key as secret-free metadata, newest first.
func ListAPIKeys(db *sql.DB) ([]APIKeyInfo, error) {
	rows, err := db.Query("SELECT id, name, prefix, last_used, create_dt FROM api_keys ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := []APIKeyInfo{}
	for rows.Next() {
		var (
			info     APIKeyInfo
			lastUsed interface{}
			created  interface{}
		)
		if err := rows.Scan(&info.ID, &info.Name, &info.Prefix, &lastUsed, &created); err != nil {
			return nil, err
		}
		if s, ok := normaliseValue(lastUsed).(string); ok {
			info.LastUsed = s
		}
		if s, ok := normaliseValue(created).(string); ok {
			info.Created = s
		}
		keys = append(keys, info)
	}
	return keys, rows.Err()
}

// VerifyAPIKey checks plaintext against every stored key whose prefix could
// match: the exact prefix, plus legacy keys carried over from the single-key
// era which have an empty prefix. On a match it records last_used, transparently
// upgrades a legacy SHA-256/plaintext hash to bcrypt, and backfills the prefix
// of a migrated key so subsequent lookups hit the indexed path. It returns true
// when the key is valid.
func VerifyAPIKey(db *sql.DB, plaintext string) (bool, error) {
	if plaintext == "" {
		return false, nil
	}

	rows, err := db.Query(
		"SELECT id, key_hash, prefix FROM api_keys WHERE prefix = $1 OR prefix = ''",
		apiKeyPrefix(plaintext),
	)
	if err != nil {
		return false, err
	}
	type candidate struct {
		id     int
		hash   string
		prefix string
	}
	var candidates []candidate
	for rows.Next() {
		var cand candidate
		if err := rows.Scan(&cand.id, &cand.hash, &cand.prefix); err != nil {
			rows.Close()
			return false, err
		}
		candidates = append(candidates, cand)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, err
	}

	for _, cand := range candidates {
		match, legacy := CheckAPIKey(plaintext, cand.hash)
		if !match {
			continue
		}

		// Upgrade a legacy hash now that the plaintext is in hand.
		if legacy {
			if newHash := HashAPIKey(plaintext); newHash != "" {
				if _, err := db.Exec("UPDATE api_keys SET key_hash = $1 WHERE id = $2", newHash, cand.id); err != nil {
					logger.Log.WithError(err).Warn("Failed to upgrade legacy API key hash")
				}
			}
		}

		// Backfill the prefix for a key migrated from the single-key era.
		if cand.prefix == "" {
			if _, err := db.Exec("UPDATE api_keys SET prefix = $1 WHERE id = $2", apiKeyPrefix(plaintext), cand.id); err != nil {
				logger.Log.WithError(err).Warn("Failed to backfill API key prefix")
			}
		}

		if _, err := db.Exec("UPDATE api_keys SET last_used = CURRENT_TIMESTAMP, update_dt = CURRENT_TIMESTAMP WHERE id = $1", cand.id); err != nil {
			logger.Log.WithError(err).Warn("Failed to record API key usage")
		}
		return true, nil
	}

	return false, nil
}

// GetAPIKeysHandler returns the secret-free list of configured keys.
func GetAPIKeysHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "GetAPIKeysHandler")
	db := DBFromContext(c)

	keys, err := ListAPIKeys(db)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to list API keys")
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"keys": keys})
}

// CreateAPIKeyHandler mints a new named key. The plaintext is returned once and
// never stored — only its bcrypt hash and prefix are persisted.
func CreateAPIKeyHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "CreateAPIKeyHandler")
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := utils.ValidateRequiredString("name", req.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	db := DBFromContext(c)

	// Names must be unique (case-insensitive) so keys stay distinguishable in
	// the list and so a regenerate/revoke can't be aimed at the wrong one.
	var existing int
	if err := db.QueryRow("SELECT COUNT(*) FROM api_keys WHERE LOWER(name) = LOWER($1)", req.Name).Scan(&existing); err != nil {
		fieldLogger.WithError(err).Error("Failed to check for duplicate API key name")
		apiInternalError(c, "api_database_error")
		return
	}
	if existing > 0 {
		apiError(c, http.StatusConflict, "api_api_key_name_exists")
		return
	}

	plaintext := GenerateAPIKey()
	hash := HashAPIKey(plaintext)
	if plaintext == "" || hash == "" {
		apiInternalError(c, "api_failed_to_save_api_key")
		return
	}

	var id int
	err := db.QueryRow(
		"INSERT INTO api_keys (name, key_hash, prefix) VALUES ($1, $2, $3) RETURNING id",
		req.Name, hash, apiKeyPrefix(plaintext),
	).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to create API key")
		apiInternalError(c, "api_failed_to_save_api_key")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": T(c, "api_api_key_generated"),
		"api_key": plaintext,
		"key": APIKeyInfo{
			ID:     id,
			Name:   req.Name,
			Prefix: apiKeyPrefix(plaintext),
		},
	})
}

// RegenerateAPIKeyHandler rotates an existing key in place: the name is kept but
// a fresh secret is issued, invalidating the previous value. The new plaintext
// is returned once.
func RegenerateAPIKeyHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "RegenerateAPIKeyHandler")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		apiBadRequest(c, "api_invalid_request")
		return
	}

	db := DBFromContext(c)
	var name string
	if err := db.QueryRow("SELECT name FROM api_keys WHERE id = $1", id).Scan(&name); err != nil {
		if err == sql.ErrNoRows {
			apiNotFound(c, "api_invalid_request")
			return
		}
		fieldLogger.WithError(err).Error("Failed to look up API key")
		apiInternalError(c, "api_database_error")
		return
	}

	plaintext := GenerateAPIKey()
	hash := HashAPIKey(plaintext)
	if plaintext == "" || hash == "" {
		apiInternalError(c, "api_failed_to_save_api_key")
		return
	}

	_, err = db.Exec(
		"UPDATE api_keys SET key_hash = $1, prefix = $2, last_used = NULL, update_dt = CURRENT_TIMESTAMP WHERE id = $3",
		hash, apiKeyPrefix(plaintext), id,
	)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to regenerate API key")
		apiInternalError(c, "api_failed_to_save_api_key")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": T(c, "api_api_key_generated"),
		"api_key": plaintext,
		"key": APIKeyInfo{
			ID:     id,
			Name:   name,
			Prefix: apiKeyPrefix(plaintext),
		},
	})
}

// RevokeAPIKeyHandler permanently deletes a key. Any system still presenting it
// is locked out immediately.
func RevokeAPIKeyHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "RevokeAPIKeyHandler")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		apiBadRequest(c, "api_invalid_request")
		return
	}

	db := DBFromContext(c)
	res, err := db.Exec("DELETE FROM api_keys WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to revoke API key")
		apiInternalError(c, "api_database_error")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		apiNotFound(c, "api_invalid_request")
		return
	}

	apiOK(c, "api_api_key_revoked")
}
