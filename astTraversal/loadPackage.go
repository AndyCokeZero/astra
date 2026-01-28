package astTraversal

import (
	"fmt"
	"sync"

	"golang.org/x/tools/go/packages"
)

const (
	fullLoadMode = packages.NeedName |
		packages.NeedFiles |
		packages.NeedImports |
		packages.NeedExportFile |
		packages.NeedTypes |
		packages.NeedSyntax |
		packages.NeedTypesInfo |
		packages.NeedTypesSizes |
		packages.NeedModule
	lightLoadMode = packages.NeedName |
		packages.NeedFiles |
		packages.NeedImports |
		packages.NeedExportFile |
		packages.NeedTypes |
		packages.NeedTypesSizes |
		packages.NeedModule
)

var (
	cachedPackages   = make(map[string]*packages.Package)
	cachedPackagesMu sync.Mutex
)

// LoadPackage loads a package from a path using the full load mode.
// Because of the way the packages.Load function works, we cache the packages to avoid loading the same package multiple times.
func LoadPackage(pkgPath string, workDir string) (*packages.Package, error) {
	return LoadPackageWithMode(pkgPath, workDir, fullLoadMode)
}

// LoadPackageWithMode loads a package from a path with the specified load mode.
func LoadPackageWithMode(pkgPath string, workDir string, mode packages.LoadMode) (*packages.Package, error) {
	cacheKey := fmt.Sprintf("%s|%d", pkgPath, mode)
	cachedPackagesMu.Lock()
	if pkg, ok := cachedPackages[cacheKey]; ok {
		cachedPackagesMu.Unlock()
		return pkg, nil
	}
	cachedPackagesMu.Unlock()

	pkg, err := LoadPackageNoCache(pkgPath, workDir, mode)
	if err != nil {
		return nil, err
	}

	cachedPackagesMu.Lock()
	cachedPackages[cacheKey] = pkg
	cachedPackagesMu.Unlock()

	return pkg, nil
}

// LoadPackageNoCache loads a package from a path.
// This function will not use the cache when loading the package.
func LoadPackageNoCache(pkgPath string, workDir string, mode packages.LoadMode) (*packages.Package, error) {
	pkgs, err := packages.Load(&packages.Config{
		Mode: mode,
		Dir:  workDir,
	}, pkgPath)
	if err != nil {
		return nil, err
	}

	for _, pkg := range pkgs {
		for _, pkgErr := range pkg.Errors {
			switch pkgErr.Kind {
			case packages.ListError:
				return nil, fmt.Errorf("package %s has list errors", pkgPath)
			case packages.TypeError:
				return nil, fmt.Errorf("package %s has type errors", pkgPath)
			case packages.ParseError:
				return nil, fmt.Errorf("package %s has parse errors", pkgPath)
			case packages.UnknownError:
				return nil, fmt.Errorf("package %s has unknown errors", pkgPath)
			}
		}
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("package %s not found", pkgPath)
	}

	return pkgs[0], nil
}
