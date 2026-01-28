package gin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/ls6-events/astra"

	"github.com/gin-gonic/gin"
)

type routeIndexLocation struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

var (
	routeIndexOnce sync.Once
	routeIndex     map[string]routeIndexLocation
)

func lookupRouteIndex(name string) (routeIndexLocation, bool) {
	if name == "" {
		return routeIndexLocation{}, false
	}

	index := loadRouteIndex()
	if index == nil {
		return routeIndexLocation{}, false
	}

	normalized := strings.TrimSpace(name)
	normalized = strings.TrimSuffix(normalized, "-fm")
	if loc, ok := index[normalized]; ok {
		return loc, true
	}
	if loc, ok := index[name]; ok {
		return loc, true
	}

	return routeIndexLocation{}, false
}

func loadRouteIndex() map[string]routeIndexLocation {
	routeIndexOnce.Do(func() {
		path := os.Getenv("ASTRA_ROUTE_INDEX_PATH")
		if path == "" {
			path = filepath.Join("resources", "astra_route_index.json")
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return
		}

		var index map[string]routeIndexLocation
		if err := json.Unmarshal(data, &index); err != nil {
			return
		}

		routeIndex = index
	})

	return routeIndex
}

// CreateRoutes creates routes from a gin routes.
// It will only create the routes and refer to the handler function by name, file and line number.
// The routes will be populated later by parseRoutes.
// It will individually call createRoute for each route.
func CreateRoutes(router *gin.Engine) astra.ServiceFunction {
	return func(s *astra.Service) error {
		s.Log.Debug().Msg("Populating service with gin routes")
		for _, route := range router.Routes() {
			s.Log.Debug().Str("path", route.Path).Str("method", route.Method).Msg("Populating route")

			denied := false
			for _, denyFunc := range s.PathDenyList {
				if denyFunc(route.Path) {
					s.Log.Debug().Str("path", route.Path).Str("method", route.Method).Msg("Path is blacklisted")
					denied = true
					break
				}
			}
			if denied {
				continue
			}

			pc := reflect.ValueOf(route.HandlerFunc).Pointer()
			runtimeFunc := runtime.FuncForPC(pc)
			file := ""
			line := 0
			if runtimeFunc != nil {
				file, line = runtimeFunc.FileLine(pc)
				if loc, ok := lookupRouteIndex(runtimeFunc.Name()); ok {
					file = loc.File
					line = loc.Line
				}
			}

			s.Log.Debug().Str("path", route.Path).Str("method", route.Method).Str("file", file).Int("line", line).Msg("Found route handler")

			s.Log.Debug().Str("path", route.Path).Str("method", route.Method).Str("file", file).Int("line", line).Msg("Parsing route")
			err := createRoute(s, file, line, route)
			if err != nil {
				s.Log.Error().Str("path", route.Path).Str("method", route.Method).Str("file", file).Int("line", line).Err(err).Msg("Failed to parse route")
				return err
			}
		}
		s.Log.Debug().Msg("Populated service with gin routes")

		return nil
	}
}
