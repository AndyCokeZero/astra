package gin

import (
	"errors"
	"go/ast"
	"go/types"
	"net/http"
	"strings"

	"github.com/ls6-events/astra"
	"github.com/ls6-events/astra/astTraversal"
)

const (
	// GinPackagePath is the import path of the gin package.
	GinPackagePath = "github.com/gin-gonic/gin"
	// GinContextType is the type of the context variable.
	GinContextType = "Context"
	// GinContextIsPointer is whether the context variable is a pointer for the handler functions.
	GinContextIsPointer = true
)

// parseFunction parses a function and adds it to the service.
// It is designed to be called recursively should it be required.
// The level parameter is used to determine the depth of recursion.
// And the package name and path are used to determine the package of the currently analysed function.
// The currRoute reference is used to manipulate the current route being analysed.
// The imports are used to determine the package of the context variable.
func parseFunction(s *astra.Service, funcTraverser *astTraversal.FunctionTraverser, currRoute *astra.Route, activeFile *astTraversal.FileNode, level int) error {
	if funcTraverser == nil || funcTraverser.Node == nil || funcTraverser.Node.Body == nil {
		if funcTraverser != nil && funcTraverser.Traverser != nil && funcTraverser.Traverser.Log != nil {
			fileName := ""
			if activeFile != nil {
				fileName = activeFile.FileName
			}
			funcTraverser.Traverser.Log.Error().
				Str("func", funcTraverser.Name()).
				Str("file", fileName).
				Msg("Function body is nil")
		}
		return errors.New("function body is nil")
	}
	traverser := funcTraverser.Traverser

	traverser.SetActiveFile(activeFile)
	traverser.SetAddComponentFunction(addComponent(s))
	var (
		callExprCount      int
		ctxArgCallCount    int
		ctxMethodCallCount int
		returnTypeCount    int
		funcTypeErrorCount int
		funcResolveErrors  []string
	)
	log := traverser.Log
	funcName := ""
	if funcTraverser != nil && funcTraverser.DeclNode != nil {
		funcName = funcTraverser.Name()
	}

	if level == 0 {
		funcDoc, err := funcTraverser.Doc()
		if err != nil {
			return err
		}
		if funcDoc != "" {
			currRoute.Doc = strings.TrimSpace(funcDoc)
		}
		if log != nil {
			log.Info().
				Str("func", funcName).
				Str("file", activeFile.FileName).
				Str("path", currRoute.Path).
				Str("method", currRoute.Method).
				Msg("Parsing handler function")
		}
	}

	ctxName := funcTraverser.FindArgumentNameByType(GinContextType, GinPackagePath, GinContextIsPointer)
	if ctxName == "" {
		if log != nil {
			fileName := ""
			if activeFile != nil {
				fileName = activeFile.FileName
			}
			log.Error().
				Str("func", funcName).
				Str("file", fileName).
				Msg("Context argument not found in function")
		}
		return errors.New("failed to find context variable name")
	}

	var err error
	// Loop over every statement in the function
	ast.Inspect(funcTraverser.Node.Body, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		resetActiveFile := func() {
			if activeFile != nil {
				traverser.SetActiveFile(activeFile)
			}
		}
		// If a function is called
		var callExpr *astTraversal.CallExpressionTraverser
		callExpr, err = traverser.CallExpression(n)
		if errors.Is(err, astTraversal.ErrInvalidNodeType) {
			err = nil
			return true
		} else if err != nil {
			return true
		}
		callExprCount++
		if shouldSkipCall(callExpr) {
			return true
		}

		funcBuilder := astra.NewContextFuncBuilder(currRoute, callExpr)

		// Loop over every custom function
		// If the custom function returns a route, use that route instead of the current route
		// And break out of this AST traversal for this call expression
		// Otherwise, continue on
		var shouldBreak bool
		for _, customFunc := range s.CustomFuncs {
			var newRoute *astra.Route
			newRoute, err = customFunc(ctxName, funcBuilder)
			if err != nil {
				return false
			}
			if newRoute != nil {
				currRoute = newRoute
				shouldBreak = true
				break
			}
		}
		if shouldBreak {
			return true
		}

		// If the function takes the context as any argument, traverse it
		_, ok := callExpr.ArgIndex(ctxName)
		if ok {
			ctxArgCallCount++
			var function *astTraversal.FunctionTraverser
			function, err = callExpr.Function()
			if err != nil {
				if log != nil {
					log.Debug().Err(err).Str("call", callExprName(callExpr)).Msg("failed to get function")
					funcResolveErrors = append(funcResolveErrors, "function:"+callExprName(callExpr))
				}
				resetActiveFile()
				return true
			}

			err = parseFunction(s, function, currRoute, function.Traverser.ActiveFile(), level+1)
			if err != nil {
				if log != nil {
					log.Debug().Err(err).Str("call", callExprName(callExpr)).Msg("error parsing function")
				}
				resetActiveFile()
				return true
			}

			resetActiveFile()
		} else {
			var funcType *types.Func
			funcType, err = callExpr.Type()
			if err != nil {
				funcTypeErrorCount++
				if log != nil {
					log.Debug().Err(err).Str("call", callExprName(callExpr)).Msg("failed to get call expression type")
					funcResolveErrors = append(funcResolveErrors, "type:"+callExprName(callExpr))
				}
				resetActiveFile()
				return true
			}

			signature, ok := funcType.Type().(*types.Signature)
			if !ok {
				if log != nil {
					log.Debug().Str("call", callExprName(callExpr)).Msg("error getting function signature")
				}
				resetActiveFile()
				return true
			}

			signaturePath := GinPackagePath + "." + GinContextType
			if GinContextIsPointer {
				signaturePath = "*" + signaturePath
			}

			if signature.Recv() != nil && signature.Recv().Type().String() == signaturePath {
				ctxMethodCallCount++
				switch funcType.Name() {
				case "JSON":
					currRoute, err = funcBuilder.StatusCode().ExpressionResult().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						statusCode, ok := params[0].(int)
						if !ok {
							return nil, errors.New("failed to parse status code")
						}

						result, ok := params[1].(astTraversal.Result)
						if !ok {
							return nil, errors.New("failed to parse result")
						}

						returnType := astra.ReturnType{
							StatusCode:  statusCode,
							ContentType: "application/json",
							Field:       astra.ParseResultToField(result),
						}

						route.ReturnTypes = astra.AddReturnType(route.ReturnTypes, returnType)
						returnTypeCount++

						return route, nil
					})
					if err != nil {
						if log != nil {
							log.Error().Err(err).Str("call", callExprName(callExpr)).Msg("failed to parse JSON return type")
						}
						return false
					}
				case "XML":
					currRoute, err = funcBuilder.StatusCode().ExpressionResult().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						statusCode, ok := params[0].(int)
						if !ok {
							return nil, errors.New("failed to parse status code")
						}

						result, ok := params[1].(astTraversal.Result)
						if !ok {
							return nil, errors.New("failed to parse result")
						}

						returnType := astra.ReturnType{
							StatusCode:  statusCode,
							ContentType: "application/xml",
							Field:       astra.ParseResultToField(result),
						}

						route.ReturnTypes = astra.AddReturnType(route.ReturnTypes, returnType)
						returnTypeCount++

						return route, nil
					})
					if err != nil {
						if log != nil {
							log.Error().Err(err).Str("call", callExprName(callExpr)).Msg("failed to parse XML return type")
						}
						return false
					}
				case "YAML":
					currRoute, err = funcBuilder.StatusCode().ExpressionResult().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						statusCode, ok := params[0].(int)
						if !ok {
							return nil, errors.New("failed to parse status code")
						}

						result, ok := params[1].(astTraversal.Result)
						if !ok {
							return nil, errors.New("failed to parse result")
						}

						returnType := astra.ReturnType{
							StatusCode:  statusCode,
							ContentType: "application/yaml",
							Field:       astra.ParseResultToField(result),
						}

						route.ReturnTypes = astra.AddReturnType(route.ReturnTypes, returnType)
						returnTypeCount++

						return route, nil
					})
					if err != nil {
						if log != nil {
							log.Error().Err(err).Str("call", callExprName(callExpr)).Msg("failed to parse YAML return type")
						}
						return false
					}
				case "ProtoBuf":
					currRoute, err = funcBuilder.StatusCode().ExpressionResult().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						statusCode, ok := params[0].(int)
						if !ok {
							return nil, errors.New("failed to parse status code")
						}

						result, ok := params[1].(astTraversal.Result)
						if !ok {
							return nil, errors.New("failed to parse result")
						}

						returnType := astra.ReturnType{
							StatusCode:  statusCode,
							ContentType: "application/protobuf",
							Field:       astra.ParseResultToField(result),
						}

						route.ReturnTypes = astra.AddReturnType(route.ReturnTypes, returnType)
						returnTypeCount++

						return route, nil
					})
					if err != nil {
						if log != nil {
							log.Error().Err(err).Str("call", callExprName(callExpr)).Msg("failed to parse ProtoBuf return type")
						}
						return false
					}
				case "Data":
					currRoute, err = funcBuilder.StatusCode().Ignored().ExpressionResult().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						statusCode, ok := params[0].(int)
						if !ok {
							return nil, errors.New("failed to parse status code")
						}

						result, ok := params[1].(astTraversal.Result)
						if !ok {
							return nil, errors.New("failed to parse result")
						}

						returnType := astra.ReturnType{
							StatusCode: statusCode,
							Field:      astra.ParseResultToField(result),
						}

						route.ReturnTypes = astra.AddReturnType(route.ReturnTypes, returnType)
						returnTypeCount++

						return route, nil
					})
					if err != nil {
						if log != nil {
							log.Error().Err(err).Str("call", callExprName(callExpr)).Msg("failed to parse Data return type")
						}
						return false
					}
				case "String": // c.String
					currRoute, err = funcBuilder.StatusCode().Ignored().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						statusCode, ok := params[0].(int)
						if !ok {
							return nil, errors.New("failed to parse status code")
						}

						returnType := astra.ReturnType{
							StatusCode:  statusCode,
							ContentType: "text/plain",
							Field: astra.Field{
								Type: "string",
							},
						}

						route.ReturnTypes = astra.AddReturnType(route.ReturnTypes, returnType)
						returnTypeCount++

						return route, nil
					})
					if err != nil {
						if log != nil {
							log.Error().Err(err).Str("call", callExprName(callExpr)).Msg("failed to parse String return type")
						}
						return false
					}
				case "Status": // c.Status
					currRoute, err = funcBuilder.StatusCode().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						statusCode, ok := params[0].(int)
						if !ok {
							return nil, errors.New("failed to parse status code")
						}

						returnType := astra.ReturnType{
							StatusCode: statusCode,
							Field: astra.Field{
								Type: "nil",
							},
						}

						route.ReturnTypes = astra.AddReturnType(route.ReturnTypes, returnType)
						returnTypeCount++

						return route, nil
					})
				// Query Param methods
				case "GetQuery", "Query":
					currRoute, err = funcBuilder.Value().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						name, ok := params[0].(string)
						if !ok {
							return nil, errors.New("failed to parse name")
						}

						param := astra.Param{
							Field: astra.Field{
								Type: "string",
							},
							Name: name,
						}

						route.QueryParams = append(route.QueryParams, param)

						return route, nil
					})
					if err != nil {
						return false
					}
				case "GetQueryArray", "QueryArray":
					currRoute, err = funcBuilder.Value().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						name, ok := params[0].(string)
						if !ok {
							return nil, errors.New("failed to parse name")
						}

						param := astra.Param{
							Field: astra.Field{
								Type: "string",
							},
							Name:    name,
							IsArray: true,
						}

						route.QueryParams = append(route.QueryParams, param)

						return route, nil
					})
					if err != nil {
						return false
					}
				case "GetQueryMap", "QueryMap":
					currRoute, err = funcBuilder.Value().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						name, ok := params[0].(string)
						if !ok {
							return nil, errors.New("failed to parse name")
						}

						param := astra.Param{
							Field: astra.Field{
								Type: "string",
							},
							Name:  name,
							IsMap: true,
						}

						route.QueryParams = append(route.QueryParams, param)

						return route, nil
					})
					if err != nil {
						return false
					}
				case "ShouldBindQuery", "BindQuery":
					currRoute, err = funcBuilder.ExpressionResult().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						result, ok := params[0].(astTraversal.Result)
						if !ok {
							return nil, errors.New("failed to parse result")
						}

						field := astra.ParseResultToField(result)

						route.QueryParams = append(route.QueryParams, astra.Param{
							IsBound: true,
							Field:   field,
						})

						return route, nil
					})
					if err != nil {
						return false
					}

				// Body Param methods
				case "ShouldBind", "Bind":
					currRoute, err = funcBuilder.ExpressionResult().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						result, ok := params[0].(astTraversal.Result)
						if !ok {
							return nil, errors.New("failed to parse result")
						}

						field := astra.ParseResultToField(result)

						route.QueryParams = append(route.QueryParams, astra.Param{
							IsBound: true,
							Field:   field,
						})

						for _, bodyBindingTag := range []astTraversal.BindingTagType{astTraversal.FormBindingTag, astTraversal.JSONBindingTag, astTraversal.XMLBindingTag, astTraversal.YAMLBindingTag} {
							contentTypes := astra.BindingTagToContentTypes(bodyBindingTag)

							for _, contentType := range contentTypes {
								route.Body = append(route.Body, astra.BodyParam{
									ContentType: contentType,
									IsBound:     true,
									Field:       field,
								})
							}
						}

						return route, nil
					})
					if err != nil {
						return false
					}
				case "ShouldBindJSON", "BindJSON":
					currRoute, err = funcBuilder.ExpressionResult().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						result, ok := params[0].(astTraversal.Result)
						if !ok {
							return nil, errors.New("failed to parse result")
						}

						field := astra.ParseResultToField(result)

						route.Body = append(route.Body, astra.BodyParam{
							ContentType: "application/json",
							IsBound:     true,
							Field:       field,
						})

						return route, nil
					})
					if err != nil {
						return false
					}
				case "ShouldBindXML", "BindXML":
					currRoute, err = funcBuilder.ExpressionResult().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						result, ok := params[0].(astTraversal.Result)
						if !ok {
							return nil, errors.New("failed to parse result")
						}

						field := astra.ParseResultToField(result)

						route.Body = append(route.Body, astra.BodyParam{
							ContentType: "application/xml",
							IsBound:     true,
							Field:       field,
						})

						return route, nil
					})
					if err != nil {
						return false
					}
				case "ShouldBindYAML", "BindYAML":
					currRoute, err = funcBuilder.ExpressionResult().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						result, ok := params[0].(astTraversal.Result)
						if !ok {
							return nil, errors.New("failed to parse result")
						}

						field := astra.ParseResultToField(result)

						route.Body = append(route.Body, astra.BodyParam{
							ContentType: "application/yaml",
							IsBound:     true,
							Field:       field,
						})

						return route, nil
					})
					if err != nil {
						return false
					}
				case "GetPostForm", "PostForm":
					currRoute, err = funcBuilder.Value().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						name, ok := params[0].(string)
						if !ok {
							return nil, errors.New("failed to parse name")
						}

						param := astra.BodyParam{
							ContentType: "application/x-www-form-urlencoded",
							Field: astra.Field{
								Type: "string",
							},
							Name: name,
						}

						route.Body = append(route.Body, param)

						return route, nil
					})
					if err != nil {
						return false
					}
				case "GetPostFormArray", "PostFormArray":
					currRoute, err = funcBuilder.Value().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						name, ok := params[0].(string)
						if !ok {
							return nil, errors.New("failed to parse name")
						}

						param := astra.BodyParam{
							ContentType: "application/x-www-form-urlencoded",
							Field: astra.Field{
								Type: "string",
							},
							Name:    name,
							IsArray: true,
						}

						route.Body = append(route.Body, param)

						return route, nil
					})
					if err != nil {
						return false
					}
				case "GetPostFormMap", "PostFormMap":
					currRoute, err = funcBuilder.Value().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						name, ok := params[0].(string)
						if !ok {
							return nil, errors.New("failed to parse name")
						}

						param := astra.BodyParam{
							ContentType: "application/x-www-form-urlencoded",
							Field: astra.Field{
								Type: "string",
							},
							Name:  name,
							IsMap: true,
						}

						route.Body = append(route.Body, param)

						return route, nil
					})
					if err != nil {
						return false
					}
				case "FormFile":
					currRoute, err = funcBuilder.Value().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						name, ok := params[0].(string)
						if !ok {
							return nil, errors.New("failed to parse name")
						}

						param := astra.BodyParam{
							ContentType: "multipart/form-data",
							Field: astra.Field{
								Type: "file",
							},
							Name: name,
						}

						route.Body = append(route.Body, param)

						return route, nil
					})
					if err != nil {
						return false
					}
				case "GetHeader":
					currRoute, err = funcBuilder.Value().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						name, ok := params[0].(string)
						if !ok {
							return nil, errors.New("failed to parse name")
						}

						param := astra.Param{
							Field: astra.Field{
								Type: "string",
							},
							Name: name,
						}

						route.RequestHeaders = append(route.RequestHeaders, param)

						return route, nil
					})
					if err != nil {
						return false
					}
				case "ShouldBindHeader", "BindHeader":
					currRoute, err = funcBuilder.ExpressionResult().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						result, ok := params[0].(astTraversal.Result)
						if !ok {
							return nil, errors.New("failed to parse result")
						}

						field := astra.ParseResultToField(result)

						route.RequestHeaders = append(route.RequestHeaders, astra.Param{
							IsBound: true,
							Field:   field,
						})

						return route, nil
					})
					if err != nil {
						return false
					}
				case "Header":
					currRoute, err = funcBuilder.Value().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						name, ok := params[0].(string)
						if !ok {
							return nil, errors.New("failed to parse name")
						}

						param := astra.Param{
							Field: astra.Field{
								Type: "string",
							},
							Name: name,
						}

						route.ResponseHeaders = append(route.ResponseHeaders, param)

						return route, nil
					})
				case "AbortWithError":
					currRoute, err = funcBuilder.StatusCode().Ignored().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						statusCode, ok := params[0].(int)
						if !ok {
							return nil, errors.New("failed to parse status code")
						}

						returnType := astra.ReturnType{
							StatusCode: statusCode,
							Field: astra.Field{
								Type: "nil",
							},
						}

						route.ReturnTypes = astra.AddReturnType(route.ReturnTypes, returnType)
						returnTypeCount++

						return route, nil
					})
					if err != nil {
						if log != nil {
							log.Error().Err(err).Str("call", callExprName(callExpr)).Msg("failed to parse AbortWithError return type")
						}
						return false
					}
				case "AbortWithStatus":
					currRoute, err = funcBuilder.StatusCode().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						statusCode, ok := params[0].(int)
						if !ok {
							return nil, errors.New("failed to parse status code")
						}

						returnType := astra.ReturnType{
							StatusCode: statusCode,
							Field: astra.Field{
								Type: "nil",
							},
						}

						route.ReturnTypes = astra.AddReturnType(route.ReturnTypes, returnType)
						returnTypeCount++

						return route, nil
					})
					if err != nil {
						if log != nil {
							log.Error().Err(err).Str("call", callExprName(callExpr)).Msg("failed to parse AbortWithStatus return type")
						}
						return false
					}
				case "AbortWithStatusJSON":
					currRoute, err = funcBuilder.StatusCode().ExpressionResult().Build(func(route *astra.Route, params []any) (*astra.Route, error) {
						statusCode, ok := params[0].(int)
						if !ok {
							return nil, errors.New("failed to parse status code")
						}

						result, ok := params[1].(astTraversal.Result)
						if !ok {
							return nil, errors.New("failed to parse result")
						}

						returnType := astra.ReturnType{
							ContentType: "application/json",
							StatusCode:  statusCode,
							Field:       astra.ParseResultToField(result),
						}

						route.ReturnTypes = astra.AddReturnType(route.ReturnTypes, returnType)
						returnTypeCount++

						return route, nil
					})
					if err != nil {
						if log != nil {
							log.Error().Err(err).Str("call", callExprName(callExpr)).Msg("failed to parse AbortWithStatusJSON return type")
						}
						return false
					}
				}
			}
			resetActiveFile()
		}

		return true
	})

	if err != nil {
		return err
	}

	if level == 0 {
		fileName := ""
		if activeFile != nil {
			fileName = activeFile.FileName
		}
		path := ""
		method := ""
		if currRoute != nil {
			path = currRoute.Path
			method = currRoute.Method
		}
		if currRoute == nil {
			if log != nil {
				log.Error().
					Str("func", funcName).
					Str("file", fileName).
					Str("path", path).
					Str("method", method).
					Msg("Current route is nil when checking return types")
			}
			return errors.New("current route is nil")
		}
		if len(currRoute.ReturnTypes) == 0 && log != nil {
			log.Warn().
				Str("func", funcName).
				Str("file", fileName).
				Str("path", path).
				Str("method", method).
				Str("ctxName", ctxName).
				Int("callExprCount", callExprCount).
				Int("ctxArgCallCount", ctxArgCallCount).
				Int("ctxMethodCallCount", ctxMethodCallCount).
				Int("returnTypeCount", returnTypeCount).
				Int("funcTypeErrorCount", funcTypeErrorCount).
				Strs("funcResolveErrors", funcResolveErrors).
				Msg("No return types found for route, falling back to empty JSON response")
		}
		if len(currRoute.ReturnTypes) == 0 {
			currRoute.ReturnTypes = astra.AddReturnType(currRoute.ReturnTypes, astra.ReturnType{
				StatusCode:  http.StatusOK,
				ContentType: "application/json",
				Field: astra.Field{
					Type: "struct",
				},
			})
		}
	}

	return nil
}

func callExprName(callExpr *astTraversal.CallExpressionTraverser) string {
	if callExpr == nil || callExpr.Node == nil || callExpr.Node.Fun == nil {
		return ""
	}

	switch nodeFun := callExpr.Node.Fun.(type) {
	case *ast.Ident:
		return nodeFun.Name
	case *ast.SelectorExpr:
		if ident, ok := nodeFun.X.(*ast.Ident); ok {
			return ident.Name + "." + nodeFun.Sel.Name
		}
		return nodeFun.Sel.Name
	default:
		return ""
	}
}

func shouldSkipCall(callExpr *astTraversal.CallExpressionTraverser) bool {
	if callExpr == nil || callExpr.Node == nil || callExpr.Node.Fun == nil {
		return false
	}
	sel, ok := callExpr.Node.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if sel.Sel != nil && sel.Sel.Name == "Translate" && isI18nServiceSelector(sel) {
		return true
	}
	if isHttputilSelector(callExpr, sel) {
		return true
	}
	return false
}

func isI18nServiceSelector(sel *ast.SelectorExpr) bool {
	if sel == nil {
		return false
	}
	switch x := sel.X.(type) {
	case *ast.Ident:
		return x.Name == "i18nService" || x.Name == "I18nService"
	case *ast.SelectorExpr:
		if x.Sel == nil {
			return false
		}
		return x.Sel.Name == "i18nService" || x.Sel.Name == "I18nService"
	default:
		return false
	}
}

func isHttputilSelector(callExpr *astTraversal.CallExpressionTraverser, sel *ast.SelectorExpr) bool {
	if sel == nil {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	if ident.Name == "httputil" {
		return true
	}
	if callExpr == nil || callExpr.File == nil {
		return false
	}
	importInfo, ok := callExpr.File.FindImport(ident.Name)
	if !ok {
		return false
	}
	pkgPath := importInfo.Package.Path()
	return strings.HasSuffix(pkgPath, "/httputil")
}

func addComponent(s *astra.Service) func(astTraversal.Result) error {
	return func(result astTraversal.Result) error {
		field := astra.ParseResultToField(result)

		if field.Package != "" {
			s.Components = astra.AddComponent(s.Components, field)
		}
		return nil
	}
}
