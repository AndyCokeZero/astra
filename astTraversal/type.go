package astTraversal

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"strconv"
	"strings"
)

type TypeTraverser struct {
	Traverser *BaseTraverser
	Node      types.Type
	Package   *PackageNode
	// name is the name of the type, if it comes from a types.Named
	// If it's not a types.Named, it's empty
	name string
}

func (t *BaseTraverser) Type(node types.Type, packageNode *PackageNode) *TypeTraverser {
	return &TypeTraverser{
		Traverser: t,
		Node:      node,
		Package:   packageNode,
	}
}

func (t *TypeTraverser) SetName(name string) *TypeTraverser {
	t.name = name
	return t
}

func (t *TypeTraverser) Result() (Result, error) {
	defer func() {
		if r := recover(); r != nil {
			if t != nil && t.Traverser != nil && t.Traverser.Log != nil {
				nodeType := ""
				if t.Node != nil {
					nodeType = reflect.TypeOf(t.Node).String()
				}
				pkgPath := ""
				if t.Package != nil {
					pkgPath = t.Package.Path()
				}
				trace := make([]string, 0)
				if t.Traverser.typeTrace != nil {
					trace = append(trace, t.Traverser.typeTrace...)
				}
				t.Traverser.Log.Error().
					Interface("panic", r).
					Str("type", typeTraceLabel(t)).
					Str("nodeType", nodeType).
					Str("package", pkgPath).
					Strs("typeTrace", trace).
					Msg("Panic while resolving type")
			}
			panic(r)
		}
	}()

	cacheKey := typeCacheKey(t)
	if t.Traverser != nil && cacheKey != "" {
		if cached, ok := t.Traverser.typeResultCache[cacheKey]; ok {
			return cached, nil
		}
	}
	traceLabel := typeTraceLabel(t)
	if t.Traverser != nil {
		if traceLabel != "" {
			for _, existing := range t.Traverser.typeTrace {
				if existing == traceLabel {
					logTypeRecursion(t.Traverser, traceLabel)
					if refResult, ok := recursionResult(t); ok {
						if cacheKey != "" {
							t.Traverser.typeResultCache[cacheKey] = refResult
						}
						return refResult, nil
					}
					fallback := Result{Type: "any", Package: t.Package}
					if cacheKey != "" {
						t.Traverser.typeResultCache[cacheKey] = fallback
					}
					return fallback, nil
				}
			}
		}
		if t.Traverser.typeTraceLimit > 0 && len(t.Traverser.typeTrace) >= t.Traverser.typeTraceLimit {
			logTypeRecursionLimit(t.Traverser)
			if refResult, ok := recursionResult(t); ok {
				if cacheKey != "" {
					t.Traverser.typeResultCache[cacheKey] = refResult
				}
				return refResult, nil
			}
			fallback := Result{Type: "any", Package: t.Package}
			if cacheKey != "" {
				t.Traverser.typeResultCache[cacheKey] = fallback
			}
			return fallback, nil
		}
		t.Traverser.typeTrace = append(t.Traverser.typeTrace, traceLabel)
		defer func() {
			if len(t.Traverser.typeTrace) > 0 {
				t.Traverser.typeTrace = t.Traverser.typeTrace[:len(t.Traverser.typeTrace)-1]
			}
		}()
	}

	var result Result
	switch n := t.Node.(type) {
	case *types.Basic:
		result = Result{
			Type:    n.Name(),
			Package: t.Package,
		}

		// If the name isn't empty, it's a named type
		// Therefore it has the potential to be an enum
		if t.name != "" && t.Traverser != nil && t.Traverser.Packages != nil && t.Package != nil {
			if t.Traverser.Packages.shouldLoadFullPackage(t.Package.Path()) {
				_, err := t.Traverser.Packages.Get(t.Package)
				if err != nil {
					return Result{}, err
				}

				// Iterate through the package's AST to find the enum values
				// We start by iterating over every file in the package
				for _, file := range t.Package.Package.Syntax {
					// Then we iterate over every declaration in the file
					for _, decl := range file.Decls {
						// If the declaration is a GenDecl, it's a const/var declaration
						if genDecl, ok := decl.(*ast.GenDecl); ok {
							// If the declaration isn't a const, we skip it (we're only looking for constants)
							if genDecl.Tok != token.CONST {
								continue
							}

							// If the declaration is a const, we iterate over every spec
							for _, spec := range genDecl.Specs {
								// If the spec is a ValueSpec, we check if the type is the same as the named type
								if valueSpec, ok := spec.(*ast.ValueSpec); ok {
									// If the type is the same as the named type, we iterate over every value
									if valueSpec.Type != nil {
										// We check this by comparing the name of the type to the name of the named type
										// It must be an Ident, otherwise it's not a named type, or it's from another package, not the one we're looking for
										if ident, ok := valueSpec.Type.(*ast.Ident); ok {
											if ident.Name == t.name {
												// We iterate over every value in the value spec
												for valueIndex, value := range valueSpec.Values {
													// If the value is a basic literal, we add it to the enum values
													if basicLit, ok := value.(*ast.BasicLit); ok {
														appendEnumName := func() {
															if valueIndex < len(valueSpec.Names) {
																result.EnumNames = append(result.EnumNames, valueSpec.Names[valueIndex].Name)
															} else {
																result.EnumNames = append(result.EnumNames, "")
															}
														}

														// Switch over the basic literal's kind to determine the type of the value
														// And format it accordingly
														switch n.Kind() {
														case types.String:
															result.EnumValues = append(result.EnumValues, strings.Trim(basicLit.Value, "\""))
															appendEnumName()
														case types.Int:
															i, err := strconv.Atoi(basicLit.Value)
															if err != nil {
																continue
															}

															result.EnumValues = append(result.EnumValues, i)
															appendEnumName()
														case types.Float32, types.Float64:
															f, err := strconv.ParseFloat(basicLit.Value, 64)
															if err != nil {
																continue
															}

															result.EnumValues = append(result.EnumValues, f)
															appendEnumName()
														case types.Bool:
															b, err := strconv.ParseBool(basicLit.Value)
															if err != nil {
																continue
															}

															result.EnumValues = append(result.EnumValues, b)
															appendEnumName()
														case types.Int8, types.Int16, types.Int32, types.Int64:
															i, err := strconv.ParseInt(basicLit.Value, 10, 64)
															if err != nil {
																continue
															}

															result.EnumValues = append(result.EnumValues, i)
															appendEnumName()
														case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
															i, err := strconv.ParseUint(basicLit.Value, 10, 64)
															if err != nil {
																continue
															}

															result.EnumValues = append(result.EnumValues, i)
															appendEnumName()
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}

	case *types.Named:
		var pkg *PackageNode
		if n.Obj().Pkg() != nil {
			pkgPath := n.Obj().Pkg().Path()
			pkg = t.Traverser.Packages.FindOrAdd(pkgPath)
			if t.Traverser != nil && t.Traverser.Packages != nil && t.Traverser.Packages.shouldLoadFullPackage(pkgPath) {
				_, err := t.Traverser.Packages.Get(pkg)
				if err != nil {
					return Result{}, err
				}
			}

			if t.Traverser.shouldAddComponent {
				namedUnderlyingResult, err := t.Traverser.Type(n.Underlying(), pkg).SetName(n.Obj().Name()).Result()
				if err != nil {
					return Result{}, err
				}

				namedUnderlyingResult.Doc, err = t.Doc()
				if err != nil {
					return Result{}, err
				}

				err = t.Traverser.addComponent(namedUnderlyingResult)
				if err != nil {
					return Result{}, err
				}
			}
		}

		result = Result{
			Type:    n.Obj().Name(),
			Package: pkg,
		}
	case *types.Pointer:
		return t.Traverser.Type(n.Elem(), t.Package).Result()
	case *types.Slice:
		sliceElemResult, err := t.Traverser.Type(n.Elem(), t.Package).Result()
		if err != nil {
			return Result{}, err
		}

		result = Result{
			Type:      "slice",
			SliceType: sliceElemResult.Type,
			Package:   sliceElemResult.Package,
		}
	case *types.Array:
		arrayElemResult, err := t.Traverser.Type(n.Elem(), t.Package).Result()
		if err != nil {
			return Result{}, err
		}

		result = Result{
			Type:        "array",
			ArrayType:   arrayElemResult.Type,
			ArrayLength: n.Len(),
			Package:     arrayElemResult.Package,
		}
	case *types.Map:
		keyResult, err := t.Traverser.Type(n.Key(), t.Package).Result()
		if err != nil {
			return Result{}, err
		}

		valueResult, err := t.Traverser.Type(n.Elem(), t.Package).Result()
		if err != nil {
			return Result{}, err
		}

		result = Result{
			Type:                "map",
			MapKeyType:          keyResult.Type,
			MapKeyPackage:       keyResult.Package,
			MapValueType:        valueResult.Type,
			MapValuePackage:     valueResult.Package,
			MapValueSliceType:   valueResult.SliceType,
			MapValueArrayType:   valueResult.ArrayType,
			MapValueArrayLength: valueResult.ArrayLength,
			Package:             valueResult.Package,
		}
	case *types.Struct:
		fields := make(map[string]Result)
		for i := 0; i < n.NumFields(); i++ {
			f := n.Field(i)
			name := f.Id()
			isExported := f.Exported()
			isEmbedded := f.Embedded()

			var bindingTag BindingTagMap
			var validationTags ValidationTagMap
			if isExported {
				bindingTag, validationTags = ParseStructTag(name, n.Tag(i))
			} else {
				continue
			}

			structFieldResult, err := t.Traverser.Type(f.Type(), t.Package).Result()
			if err != nil {
				return Result{}, err
			}

			if structFieldResult.Package != nil && t.Traverser != nil && t.Traverser.Packages != nil {
				pkgPath := structFieldResult.Package.Path()
				if t.Traverser.Packages.isLocalPackagePath(pkgPath) {
					_, err = t.Traverser.Packages.Get(structFieldResult.Package)
					if err != nil {
						return Result{}, err
					}

					pos := f.Pos()

					node, err := structFieldResult.Package.ASTAtPos(pos)
					if err == nil && node != nil {
						if field, ok := node.(*ast.Field); ok {
							fieldName := name
							if len(field.Names) > 0 && field.Names[0] != nil {
								fieldName = field.Names[0].Name
							}
							if field.Doc != nil {
								if t.Traverser != nil && t.Traverser.Log != nil {
									t.Traverser.Log.Debug().Str("field", fieldName).Msg("Found doc for field")
								}
								structFieldResult.Doc = FormatDoc(field.Doc.Text())
							}
						}
					}
				}
			}

			structFieldResult.IsEmbedded = isEmbedded
			structFieldResult.StructFieldBindingTags = bindingTag
			structFieldResult.StructFieldValidationTags = validationTags

			fields[name] = structFieldResult
		}

		result = Result{
			Type:         "struct",
			StructFields: fields,
			Package:      t.Package,
		}
	case *types.Interface:
		result = Result{
			Type:    "any",
			Package: t.Package,
		}
	}

	if t.name != "" {
		result.Name = t.name
	}

	if result.Type != "" {
		if t.Traverser != nil && cacheKey != "" {
			t.Traverser.typeResultCache[cacheKey] = result
		}
		return result, nil
	} else {
		return Result{}, ErrInvalidNodeType
	}
}

func typeTraceLabel(t *TypeTraverser) string {
	if t == nil || t.Node == nil {
		return ""
	}

	switch n := t.Node.(type) {
	case *types.Named:
		if n.Obj() != nil && n.Obj().Pkg() != nil {
			return n.Obj().Pkg().Path() + "." + n.Obj().Name()
		}
		if n.Obj() != nil {
			return n.Obj().Name()
		}
	}

	return nString(t.Node)
}

func nString(node types.Type) string {
	if node == nil {
		return ""
	}
	return node.String()
}

func recursionResult(t *TypeTraverser) (Result, bool) {
	if t == nil || t.Node == nil {
		return Result{}, false
	}

	switch n := t.Node.(type) {
	case *types.Named:
		pkg := packageNodeFromNamed(t.Traverser, n)
		return Result{
			Type:    n.Obj().Name(),
			Package: pkg,
		}, true
	case *types.Pointer:
		if named, ok := n.Elem().(*types.Named); ok {
			pkg := packageNodeFromNamed(t.Traverser, named)
			return Result{
				Type:    named.Obj().Name(),
				Package: pkg,
			}, true
		}
	case *types.Slice:
		if named, ok := n.Elem().(*types.Named); ok {
			pkg := packageNodeFromNamed(t.Traverser, named)
			return Result{
				Type:      "slice",
				SliceType: named.Obj().Name(),
				Package:   pkg,
			}, true
		}
	case *types.Array:
		if named, ok := n.Elem().(*types.Named); ok {
			pkg := packageNodeFromNamed(t.Traverser, named)
			return Result{
				Type:        "array",
				ArrayType:   named.Obj().Name(),
				ArrayLength: n.Len(),
				Package:     pkg,
			}, true
		}
	case *types.Map:
		keyName, keyPkg := typeNameAndPackage(t.Traverser, n.Key())
		valName, valPkg := typeNameAndPackage(t.Traverser, n.Elem())
		return Result{
			Type:          "map",
			MapKeyType:    keyName,
			MapKeyPackage: keyPkg,
			MapValueType:  valName,
			Package:       valPkg,
		}, true
	}

	if trace := nString(t.Node); trace != "" {
		return Result{
			Type:    trace,
			Package: t.Package,
		}, true
	}

	return Result{}, false
}

func typeNameAndPackage(traverser *BaseTraverser, node types.Type) (string, *PackageNode) {
	if node == nil {
		return "", nil
	}
	if named, ok := node.(*types.Named); ok {
		return named.Obj().Name(), packageNodeFromNamed(traverser, named)
	}
	return node.String(), nil
}

func packageNodeFromNamed(traverser *BaseTraverser, named *types.Named) *PackageNode {
	if named == nil || named.Obj() == nil || named.Obj().Pkg() == nil || traverser == nil {
		return nil
	}
	return traverser.Packages.FindOrAdd(named.Obj().Pkg().Path())
}

func logTypeRecursion(traverser *BaseTraverser, traceLabel string) {
	if traverser == nil || traverser.Log == nil {
		return
	}
	if traceLabel == "" {
		traceLabel = "unknown"
	}
	if traverser.typeRecursionLogged[traceLabel] {
		return
	}
	traverser.typeRecursionLogged[traceLabel] = true
	// traverser.Log.Error().
	// 	Str("type", traceLabel).
	// 	Strs("trace", shortenTrace(traverser.typeTrace, 8)).
	// 	Msg("Detected type recursion")
}

func logTypeRecursionLimit(traverser *BaseTraverser) {
	if traverser == nil || traverser.Log == nil {
		return
	}
	traverser.Log.Error().
		Int("limit", traverser.typeTraceLimit).
		Strs("trace", shortenTrace(traverser.typeTrace, 8)).
		Msg("Type recursion depth exceeded limit")
}

func shortenTrace(trace []string, limit int) []string {
	if limit <= 0 || len(trace) <= limit {
		return trace
	}
	out := make([]string, 0, limit+1)
	out = append(out, trace[:limit]...)
	out = append(out, "...")
	return out
}

func typeCacheKey(t *TypeTraverser) string {
	if t == nil || t.Node == nil {
		return ""
	}
	switch n := t.Node.(type) {
	case *types.Named:
		if n.Obj() != nil && n.Obj().Pkg() != nil {
			return "named:" + n.Obj().Pkg().Path() + "." + n.Obj().Name()
		}
		if n.Obj() != nil {
			return "named:" + n.Obj().Name()
		}
	}
	return "type:" + nString(t.Node)
}

func (t *TypeTraverser) Doc() (string, error) {
	if named, ok := t.Node.(*types.Named); ok {
		if t.Traverser == nil || t.Traverser.Packages == nil {
			return "", nil
		}
		pkgPath := named.Obj().Pkg().Path()
		if !t.Traverser.Packages.isLocalPackagePath(pkgPath) {
			return "", nil
		}
		pkg := t.Traverser.Packages.AddPackage(pkgPath)

		_, err := t.Traverser.Packages.Get(pkg)
		if err != nil {
			return "", err
		}

		doc, ok := pkg.FindDocForType(named.Obj().Name())
		if ok {
			return FormatDoc(doc), nil
		}
	}

	return "", nil
}
