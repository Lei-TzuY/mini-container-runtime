package image

// ApplyLayer applies one OCI/Docker filesystem-diff layer to destDir,
// including whiteout and opaque-directory semantics. Callers that stack image
// layers must use this instead of Unpack, which intentionally treats a tar as a
// plain filesystem archive and therefore does not interpret whiteouts.
func ApplyLayer(layerPath, destDir string) error {
	return applyLayer(layerPath, destDir)
}
