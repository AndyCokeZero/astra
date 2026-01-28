package astra

import (
	"fmt"
	"go/ast"
	"go/types"
	"sync"

	"golang.org/x/tools/go/packages"
)

// ScanHandlers scans Go packages and returns a HandlerLocator with all function positions.
// The workDir is the working directory for package loading.
// Patterns follow golang.org/x/tools/go/packages format (e.g., "./...", "./handlers", "github.com/myapp/handlers").
// If no patterns are provided, "./..." is used as default.
func ScanHandlers(workDir string, patterns ...string) (HandlerLocator, error) {
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedFiles,
		Dir:  workDir,
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}

	index := make(MapHandlerLocator)
	var mu sync.Mutex

	for _, pkg := range pkgs {
		if pkg == nil || len(pkg.Syntax) == 0 {
			continue
		}

		pkgPath := pkg.PkgPath
		// For main package, use "main" as the package path to match runtime.FuncForPC naming
		if pkg.Name == "main" {
			pkgPath = "main"
		}

		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				decl, ok := n.(*ast.FuncDecl)
				if !ok {
					return true
				}

				key := buildFuncKey(pkgPath, decl, pkg.TypesInfo)
				if key == "" {
					return true
				}

				pos := pkg.Fset.Position(decl.Pos())
				if pos.Filename == "" || pos.Line == 0 {
					return true
				}

				mu.Lock()
				index[key] = HandlerLocation{File: pos.Filename, Line: pos.Line}
				mu.Unlock()

				return true
			})
		}
	}

	return index, nil
}

// buildFuncKey constructs a function key matching the format used by runtime.FuncForPC.
// For regular functions: "pkgPath.FuncName"
// For methods: "pkgPath.(*ReceiverType).MethodName" or "pkgPath.(ReceiverType).MethodName"
func buildFuncKey(pkgPath string, decl *ast.FuncDecl, info *types.Info) string {
	if decl == nil || decl.Name == nil {
		return ""
	}

	// Regular function (no receiver)
	if decl.Recv == nil {
		return pkgPath + "." + decl.Name.Name
	}

	// Method with receiver
	obj, ok := info.Defs[decl.Name]
	if !ok {
		return ""
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return ""
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return ""
	}
	recv := sig.Recv()
	if recv == nil {
		return ""
	}

	recvType := recv.Type()
	isPointer := false
	if ptr, ok := recvType.(*types.Pointer); ok {
		recvType = ptr.Elem()
		isPointer = true
	}

	named, ok := recvType.(*types.Named)
	if !ok {
		return ""
	}
	typeName := named.Obj().Name()
	if typeName == "" {
		return ""
	}

	if isPointer {
		return fmt.Sprintf("%s.(*%s).%s", pkgPath, typeName, fn.Name())
	}
	return fmt.Sprintf("%s.(%s).%s", pkgPath, typeName, fn.Name())
}
