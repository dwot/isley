package handlers

// Installed themes (Settings › Appearance): add one from a package, remove
// one, and download any theme as a package. The package format and its
// checks are in the theme package.

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"isley/logger"
	"isley/theme"
)

const contextKeyThemes = "themes"

// ThemesFromContext returns the engine's theme registry.
func ThemesFromContext(c *gin.Context) *theme.Registry {
	return c.MustGet(contextKeyThemes).(*theme.Registry)
}

// SetThemesOnContext binds the registry to a request context.
func SetThemesOnContext(c *gin.Context, r *theme.Registry) { c.Set(contextKeyThemes, r) }

// themeProblem answers 400 with what is wrong with the package. The
// detail is the theme package's own wording, which names the token or the
// file at fault; it is in English whatever the page's language.
func themeProblem(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s: %s", T(c, "api_theme_invalid"), err.Error())})
}

// InstallTheme adds the theme in an uploaded package (form field
// "theme"), or replaces the installed theme with the same id.
func InstallTheme(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "InstallTheme")

	// Nothing past the package limit is read, whatever the form says.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, theme.MaxPackageSize+64<<10)
	fileHeader, err := c.FormFile("theme")
	if err != nil {
		apiBadRequest(c, "api_failed_to_retrieve_file")
		return
	}
	if fileHeader.Size > theme.MaxPackageSize {
		themeProblem(c, fmt.Errorf("the package is larger than %d MB", theme.MaxPackageSize>>20))
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		apiInternalError(c, "api_failed_to_open_file")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, theme.MaxPackageSize+1))
	if err != nil {
		apiInternalError(c, "api_failed_to_process_file")
		return
	}

	pkg, warnings, err := theme.ReadPackage(data)
	if err != nil {
		themeProblem(c, err)
		return
	}
	t, replaced, err := ThemesFromContext(c).Install(pkg)
	if err != nil {
		fieldLogger.WithError(err).Warn("Theme not installed")
		themeProblem(c, err)
		return
	}
	fieldLogger.WithField("theme", t.ID).Info("Theme installed")
	if warnings == nil {
		warnings = []string{}
	}
	lowContrast := t.LowContrast
	if lowContrast == nil {
		lowContrast = []theme.ContrastIssue{}
	}
	c.JSON(http.StatusOK, gin.H{
		"message":  T(c, "api_theme_installed"),
		"theme":    gin.H{"id": t.ID, "name": t.Name, "modes": t.Modes()},
		"replaced": replaced,
		"warnings": warnings,
		// Pairs of the theme's colours that are hard to read. Told, not
		// refused.
		"contrast": lowContrast,
	})
}

// DeleteTheme removes an installed theme. An instance that had it as its
// default goes back to Classic; a browser that had picked it falls back
// to the instance default by itself.
func DeleteTheme(c *gin.Context) {
	id := c.Param("id")
	switch err := ThemesFromContext(c).Remove(id); {
	case errors.Is(err, theme.ErrNotFound):
		apiNotFound(c, "api_theme_not_found")
		return
	case errors.Is(err, theme.ErrBuiltIn):
		apiBadRequest(c, "api_theme_built_in")
		return
	case err != nil:
		logger.Log.WithField("func", "DeleteTheme").WithError(err).Error("Failed to remove theme")
		apiInternalError(c, "api_theme_remove_failed")
		return
	}
	store := ConfigStoreFromContext(c)
	if store.Theme() == id {
		if err := UpdateSetting(DBFromContext(c), store, "theme", theme.DefaultID); err != nil {
			logger.Log.WithField("func", "DeleteTheme").WithError(err).Error("Failed to reset default theme")
		}
	}
	apiOK(c, "api_theme_removed")
}

// ExportTheme downloads a theme as a package. A built-in theme comes out
// under a new id, ready to change and install.
func ExportTheme(c *gin.Context) {
	pkg, err := ThemesFromContext(c).Export(c.Param("id"))
	if errors.Is(err, theme.ErrNotFound) {
		apiNotFound(c, "api_theme_not_found")
		return
	}
	var data []byte
	if err == nil {
		data, err = pkg.Zip()
	}
	if err != nil {
		logger.Log.WithField("func", "ExportTheme").WithError(err).Error("Failed to export theme")
		apiInternalError(c, "api_theme_export_failed")
		return
	}
	// The id is letters, digits and hyphens: safe in a header as it is.
	c.Header("Content-Disposition", `attachment; filename="`+pkg.Manifest.ID+`.isleytheme.zip"`)
	c.Data(http.StatusOK, "application/zip", data)
}

// The theme editor (Settings › Appearance › Edit) works on a manifest in
// the page. It asks CheckTheme about it as it changes and hands it to
// SaveEditedTheme when done. Both put it through the checks an uploaded
// package gets; the editor is a convenience, not a way around them.

// CheckTheme says whether a manifest (the JSON body) would be accepted,
// and which of its colour pairs are hard to read. Nothing is stored.
func CheckTheme(c *gin.Context) {
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10))
	if err != nil {
		apiBadRequest(c, "api_invalid_settings_payload")
		return
	}
	pkg, warnings, err := theme.NewPackage(data, nil, nil)
	if err != nil {
		// Not a failed request: the answer to "is this all right?" is no.
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	contrast := ThemesFromContext(c).Contrast(pkg.Manifest)
	if contrast == nil {
		contrast = []theme.ContrastIssue{}
	}
	if warnings == nil {
		warnings = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "warnings": warnings, "contrast": contrast})
}

// SaveEditedTheme installs the editor's theme. The form carries:
//
//	manifest   theme.json
//	from       the theme the editor started from; its logos are kept
//	           unless new ones come with the form
//	new        "true" for a theme being created: its id must be free
//	logo, logo_dark   optional replacement logos
func SaveEditedTheme(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "SaveEditedTheme")
	themes := ThemesFromContext(c)

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, theme.MaxPackageSize+64<<10)
	formFile := func(field string) ([]byte, error) {
		fh, err := c.FormFile(field)
		if err != nil {
			return nil, nil // not sent
		}
		f, err := fh.Open()
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return io.ReadAll(io.LimitReader(f, theme.MaxPackageSize))
	}
	manifest := c.PostForm("manifest")
	if manifest == "" {
		apiBadRequest(c, "api_invalid_settings_payload")
		return
	}
	logo, err := formFile("logo")
	if err != nil {
		apiInternalError(c, "api_failed_to_process_file")
		return
	}
	logoDark, err := formFile("logo_dark")
	if err != nil {
		apiInternalError(c, "api_failed_to_process_file")
		return
	}
	pkg, warnings, err := theme.NewPackage([]byte(manifest), logo, logoDark)
	if err != nil {
		themeProblem(c, err)
		return
	}

	from, isNew := c.PostForm("from"), c.PostForm("new") == "true"
	id := pkg.Manifest.ID
	switch {
	case isNew && themes.Has(id):
		themeProblem(c, fmt.Errorf("a theme with the id %q exists already; pick another name or id", id))
		return
	case !isNew && (from != id || themes.BuiltIn(id) || !themes.Has(id)):
		// An edit saves over the theme it opened, and only that one.
		themeProblem(c, fmt.Errorf("the theme being edited is no longer installed"))
		return
	}
	// The logos the editor did not replace come from where it started.
	if source, err := themes.Export(from); err == nil {
		if pkg.Logo == nil {
			pkg.Logo = source.Logo
		}
		if pkg.LogoDark == nil && logo == nil {
			// A new main logo without a new dark one means one logo.
			pkg.LogoDark = source.LogoDark
		}
	}

	t, _, err := themes.Install(pkg)
	if err != nil {
		fieldLogger.WithError(err).Warn("Theme not saved")
		themeProblem(c, err)
		return
	}
	fieldLogger.WithField("theme", t.ID).Info("Theme saved from the editor")
	if warnings == nil {
		warnings = []string{}
	}
	contrast := t.LowContrast
	if contrast == nil {
		contrast = []theme.ContrastIssue{}
	}
	c.JSON(http.StatusOK, gin.H{
		"message":  T(c, "api_theme_saved"),
		"theme":    gin.H{"id": t.ID, "name": t.Name, "modes": t.Modes()},
		"warnings": warnings,
		"contrast": contrast,
	})
}

// ThemeEditorData is what the editor page opens with: the manifest to
// edit, and whether saving makes a new theme. An installed theme is
// edited in place. A built-in theme is the start of a new one, under an
// id nothing has yet.
func ThemeEditorData(c *gin.Context, id string) (manifest string, from string, isNew bool, ok bool) {
	themes := ThemesFromContext(c)
	pkg, err := themes.Export(id)
	if err != nil {
		return "", "", false, false
	}
	isNew = themes.BuiltIn(id)
	if isNew {
		pkg.Manifest.ID = themes.FreeID(pkg.Manifest.ID)
	}
	data, err := pkg.Manifest.JSON()
	if err != nil {
		return "", "", false, false
	}
	return string(data), id, isNew, true
}
