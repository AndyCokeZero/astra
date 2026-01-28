package openapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/ls6-events/astra"
	"github.com/ls6-events/astra/astTraversal"
	"github.com/ls6-events/astra/utils"

	"github.com/iancoleman/strcase"
	"gopkg.in/yaml.v3"
)

func preferredComponentBinding(bindingTags []astTraversal.BindingTagType) astTraversal.BindingTagType {
	preferredOrder := []astTraversal.BindingTagType{
		astTraversal.JSONBindingTag,
		astTraversal.YAMLBindingTag,
		astTraversal.XMLBindingTag,
		astTraversal.FormBindingTag,
		astTraversal.URIBindingTag,
		astTraversal.HeaderBindingTag,
		astTraversal.NoBindingTag,
	}

	for _, preferred := range preferredOrder {
		for _, bindingTag := range bindingTags {
			if bindingTag == preferred {
				return bindingTag
			}
		}
	}

	if len(bindingTags) > 0 {
		return bindingTags[0]
	}

	return astTraversal.NoBindingTag
}

func defaultOperationID(method string, endpointPath string) string {
	raw := strings.ToLower(method) + " " + endpointPath
	sanitized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, raw)
	return strcase.ToLowerCamel(sanitized)
}

// Generate the OpenAPI output.
// It will marshal the OpenAPI struct and write it to a file.
// It will also generate the paths and their operations.
// It will also generate the components and their schemas.
func Generate(filePath string) astra.ServiceFunction {
	return func(s *astra.Service) error {
		s.Log.Debug().Msg("Generating OpenAPI output")
		if s.Config == nil {
			s.Log.Error().Msg("No config found")
			return astra.ErrConfigNotFound
		}

		s.Log.Debug().Msg("Making collision safe struct names")
		makeCollisionSafeNamesFromComponents(s.Components)

		protocol := "http"
		if s.Config.Secure {
			protocol += "s"
		}

		paths := make(Paths)
		operationIDs := make(map[string]int)
		s.Log.Debug().Msg("Adding paths")
		for _, endpoint := range s.Routes {
			s.Log.Debug().Str("endpointPath", endpoint.Path).Str("method", endpoint.Method).Msg("Generating endpoint")
			s.Log.Debug().
				Str("endpointPath", endpoint.Path).
				Str("method", endpoint.Method).
				Int("returnTypeCount", len(endpoint.ReturnTypes)).
				Msg("Preparing endpoint for responses")

			endpoint.Path = utils.MapPathParams(endpoint.Path, func(param string) string {
				if param[0] == ':' {
					return fmt.Sprintf("{%s}", param[1:])
				} else {
					return fmt.Sprintf("{%s*}", param[1:])
				}
			})

			operation := Operation{
				Responses: make(map[string]Response),
			}

			for _, pathParam := range endpoint.PathParams {
				s.Log.Debug().Str("endpointPath", endpoint.Path).Str("method", endpoint.Method).Str("param", pathParam.Name).Msg("Adding endpointPath parameter")
				schema, bound := mapParamToSchema(astTraversal.URIBindingTag, pathParam)
				if !bound {
					continue
				}
				schema = ensureSchema(schema)

				operation.Parameters = append(operation.Parameters, Parameter{
					Name:     pathParam.Name,
					In:       "path",
					Required: pathParam.IsRequired,
					Schema:   schema,
				})
			}

			for _, requestHeader := range endpoint.RequestHeaders {
				s.Log.Debug().Str("endpointPath", endpoint.Path).Str("method", endpoint.Method).Str("param", requestHeader.Name).Msg("Adding request header")
				if requestHeader.IsBound {
					field, found := findComponentByPackageAndType(s.Components, requestHeader.Field.Package, requestHeader.Field.Type)
					if !found {
						continue
					}

					component, bound := componentToSchema(s, field, astTraversal.HeaderBindingTag)
					if !bound {
						continue
					}

					for propertyName, propertySchema := range component.Properties {
						propertySchema = ensureSchema(propertySchema)
						operation.Parameters = append(operation.Parameters, Parameter{
							Name:     propertyName,
							In:       "header",
							Required: requestHeader.IsRequired,
							Schema:   propertySchema,
						})
					}
				} else {
					schema, bound := mapParamToSchema(astTraversal.HeaderBindingTag, requestHeader)
					if !bound {
						continue
					}
					schema = ensureSchema(schema)

					parameter := Parameter{
						Name:     requestHeader.Name,
						In:       "header",
						Required: requestHeader.IsRequired,
						Schema:   schema,
					}

					operation.Parameters = append(operation.Parameters, parameter)
				}
			}

			for _, queryParam := range endpoint.QueryParams {
				s.Log.Debug().Str("endpointPath", endpoint.Path).Str("method", endpoint.Method).Str("param", queryParam.Name).Msg("Adding query parameter")
				schema, bound := mapParamToSchema(astTraversal.FormBindingTag, queryParam)
				if !bound {
					continue
				}

				// OpenAPI spec requires the use of a name, so bound parameters must be spread
				if queryParam.IsBound {
					field, found := findComponentByPackageAndType(s.Components, queryParam.Field.Package, queryParam.Field.Type)
					if !found {
						continue
					}

					component, bound := componentToSchema(s, field, astTraversal.FormBindingTag)
					if !bound {
						continue
					}

					for propertyName, propertySchema := range component.Properties {
						propertySchema = ensureSchema(propertySchema)
						style, explode := getQueryParamStyle(propertySchema)

						parameter := Parameter{
							Name:     propertyName,
							In:       "query",
							Required: queryParam.IsRequired,
							Explode:  explode,
							Style:    style,
							Schema:   propertySchema,
						}

						operation.Parameters = append(operation.Parameters, parameter)
					}
				} else {
					style, explode := getQueryParamStyle(schema)

					parameter := Parameter{
						Name:     queryParam.Name,
						In:       "query",
						Required: queryParam.IsRequired,
						Explode:  explode,
						Style:    style,
						Schema:   ensureSchema(schema),
					}

					operation.Parameters = append(operation.Parameters, parameter)
				}
			}

			for _, bodyParam := range endpoint.Body {
				s.Log.Debug().Str("endpointPath", endpoint.Path).Str("method", endpoint.Method).Str("param", bodyParam.Name).Msg("Adding body parameter")
				bindingType := astra.ContentTypeToBindingTag(bodyParam.ContentType)
				schema, bound := mapFieldToSchema(bindingType, bodyParam.Field)
				if !bound {
					continue
				}

				if operation.RequestBody == nil {
					operation.RequestBody = &RequestBody{
						Content: map[string]MediaType{},
					}
				}

				var mediaType MediaType
				if bodyParam.Name != "" {
					mediaType.Schema = Schema{
						Type: "object",
						Properties: map[string]Schema{
							bodyParam.Name: schema,
						},
					}
				} else {
					mediaType.Schema = schema
				}

				operation.RequestBody.Content[bodyParam.ContentType] = mediaType
			}

			var responseHeaders map[string]Header
			if len(endpoint.ResponseHeaders) > 0 {
				responseHeaders = make(map[string]Header)
				for _, responseHeader := range endpoint.ResponseHeaders {
					s.Log.Debug().Str("endpointPath", endpoint.Path).Str("method", endpoint.Method).Str("param", responseHeader.Name).Msg("Adding response header")
					schema, bound := mapParamToSchema(astTraversal.HeaderBindingTag, responseHeader)
					if bound {
						responseHeaders[responseHeader.Name] = Header{
							Schema:   schema,
							Required: responseHeader.IsRequired,
						}
					}
				}
			}

			for _, returnType := range endpoint.ReturnTypes {
				s.Log.Debug().Str("endpointPath", endpoint.Path).Str("method", endpoint.Method).Str("return", returnType.Field.Name).Msg("Adding return type")
				var mediaType MediaType
				bindingType := astra.ContentTypeToBindingTag(returnType.ContentType)
				schema, bound := mapFieldToSchema(bindingType, returnType.Field)
				if bound {
					mediaType.Schema = schema
				}

				statusCode := strconv.Itoa(returnType.StatusCode)
				if _, set := operation.Responses[statusCode]; !set {
					operation.Responses[statusCode] = Response{
						Description: "",
						Headers:     responseHeaders,
						Content:     map[string]MediaType{},
						Links:       nil,
					}
				}

				if !reflect.DeepEqual(mediaType, MediaType{}) {
					operation.Responses[statusCode].Content[returnType.ContentType] = mediaType
				}
			}
			if len(endpoint.ReturnTypes) == 0 {
				operation.Responses["200"] = Response{
					Description: "",
					Headers:     responseHeaders,
					Content: map[string]MediaType{
						"application/json": {
							Schema: Schema{
								Type: "object",
							},
						},
					},
				}
			}
			if len(endpoint.ReturnTypes) > 0 && len(operation.Responses) == 0 {
				s.Log.Error().
					Str("endpointPath", endpoint.Path).
					Str("method", endpoint.Method).
					Msg("Return types present but responses are empty")
			}

			if endpoint.Doc != "" {
				operation.Description = endpoint.Doc
			}

			operationID := endpoint.OperationID
			if operationID == "" {
				operationID = defaultOperationID(endpoint.Method, endpoint.Path)
			}
			if operationID != "" {
				if count, ok := operationIDs[operationID]; ok {
					count++
					operationIDs[operationID] = count
					operationID = fmt.Sprintf("%s_%d", operationID, count)
				} else {
					operationIDs[operationID] = 1
				}
				operation.OperationID = operationID
			}

			// Sort parameters by name
			sort.Slice(operation.Parameters, func(i, j int) bool {
				return operation.Parameters[i].Name < operation.Parameters[j].Name
			})

			var endpointPath Path
			if _, ok := paths[endpoint.Path]; !ok {
				endpointPath = Path{}
			} else {
				endpointPath = paths[endpoint.Path]
			}
			switch endpoint.Method {
			case http.MethodGet:
				endpointPath.Get = &operation
			case http.MethodPost:
				endpointPath.Post = &operation
			case http.MethodPut:
				endpointPath.Put = &operation
			case http.MethodPatch:
				endpointPath.Patch = &operation
			case http.MethodDelete:
				endpointPath.Delete = &operation
			case http.MethodHead:
				endpointPath.Head = &operation
			case http.MethodOptions:
				endpointPath.Options = &operation
			}

			paths[endpoint.Path] = endpointPath
			s.Log.Debug().Str("path", endpoint.Path).Str("method", endpoint.Method).Msg("Added path")
		}
		s.Log.Debug().Msg("Added paths")

		components := Components{
			Schemas: make(map[string]Schema),
		}

		s.Log.Debug().Msg("Adding components")
		for _, component := range s.Components {
			addComponentSchema := func(bindingType astTraversal.BindingTagType) {
				schema, bound := componentToSchema(s, component, bindingType)
				if !bound {
					return
				}

				s.Log.Debug().Interface("binding", bindingType).Str("name", component.Name).Msg("Adding component")

				if component.Doc != "" {
					schema.Description = component.Doc
				}

				componentName, bound := makeComponentRefName(bindingType, component.Name, component.Package)
				if bound {
					components.Schemas[componentName] = schema
				}
			}

			bindingTags, uniqueBindings := astra.ExtractBindingTags(component.StructFields)
			if uniqueBindings {
				for _, bindingType := range bindingTags {
					addComponentSchema(bindingType)
				}
				continue
			}

			addComponentSchema(preferredComponentBinding(bindingTags))
		}
		s.Log.Debug().Msg("Added components")

		if s.Config.Description == "" {
			s.Config.Description = "Generated by astra"
		}

		s.Log.Debug().Msg("Generating OpenAPI schema file")
		output := OpenAPISchema{
			OpenAPI: "3.0.0",
			Info: Info{
				Title:       s.Config.Title,
				Description: s.Config.Description,
				Contact:     Contact(s.Config.Contact),
				License:     License(s.Config.License),
				Version:     s.Config.Version,
			},
			Servers: []Server{
				{
					URL: fmt.Sprintf("%s://%s:%d%s", protocol, s.Config.Host, s.Config.Port, s.Config.BasePath),
				},
			},
			Paths:      paths,
			Components: components,
		}

		if !strings.HasSuffix(filePath, ".json") && !strings.HasSuffix(filePath, ".yaml") && !strings.HasSuffix(filePath, ".yml") {
			s.Log.Debug().Msg("No file extension provided, defaulting to .json")
			filePath += ".json"
		}

		var file []byte
		var err error
		if strings.HasSuffix(filePath, ".yaml") || strings.HasSuffix(filePath, ".yml") {
			s.Log.Debug().Msg("Writing YAML file")
			file, err = yaml.Marshal(output)
		} else {
			s.Log.Debug().Msg("Writing JSON file")
			file, err = json.MarshalIndent(output, "", "  ")
		}
		if err != nil {
			s.Log.Error().Err(err).Msg("Failed to marshal OpenAPI schema")
			return err
		}

		filePath = path.Join(s.WorkDir, filePath)
		err = os.WriteFile(filePath, file, 0644)
		if err != nil {
			s.Log.Error().Err(err).Msg("Failed to write OpenAPI schema file")
			return err
		}

		s.Log.Debug().Str("filePath", filePath).Msg("Successfully generated OpenAPI schema file")

		return nil
	}
}
