package astra

// WithCustomWorkDir is an option to set the working directory of the service to a custom directory.
func WithCustomWorkDir(wd string) Option {
	return func(s *Service) {
		s.WorkDir = wd
	}
}

// WithHandlerLocator sets a custom handler locator for looking up handler source positions.
func WithHandlerLocator(locator HandlerLocator) Option {
	return func(s *Service) {
		s.HandlerLocator = locator
	}
}

// WithHandlerLocations sets handler locations from a map.
// This is a convenience function for simple use cases where handler locations are known statically.
func WithHandlerLocations(locations map[string]HandlerLocation) Option {
	return func(s *Service) {
		s.HandlerLocator = MapHandlerLocator(locations)
	}
}

// WithHandlerScanPaths scans Go packages for handler locations.
// Patterns follow golang.org/x/tools/go/packages format (e.g., "./...", "./handlers").
// If workDir is empty, the service's WorkDir will be used.
// If no patterns are provided, "./..." is used as default.
// Note: This function logs errors but does not fail if scanning fails.
func WithHandlerScanPaths(workDir string, patterns ...string) Option {
	return func(s *Service) {
		wd := workDir
		if wd == "" {
			wd = s.WorkDir
		}

		locator, err := ScanHandlers(wd, patterns...)
		if err != nil {
			s.Log.Warn().Err(err).Msg("Failed to scan handlers, falling back to runtime detection")
			return
		}
		s.HandlerLocator = locator
	}
}
