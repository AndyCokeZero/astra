package openapi

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/ls6-events/astra"
	"github.com/ls6-events/astra/astTraversal"
)

// collisionSafeNames is a map of a full name package path to a collision safe name.
var collisionSafeNames = make(map[string]string)

// collisionSafeKey creates a key for the collisionSafeNames map.
func collisionSafeKey(bindingType astTraversal.BindingTagType, name, pkg string) string {
	var keyComponents []string

	if bindingType != astTraversal.NoBindingTag {
		keyComponents = []string{pkg, string(bindingType), name}
	} else {
		keyComponents = []string{pkg, name}
	}

	return strings.Join(keyComponents, ".")
}

// getPackageName gets the package name from the package path (i.e. github.com/ls6-events/astra -> astra).
func getPackageName(pkg string) string {
	return pkg[strings.LastIndex(pkg, "/")+1:]
}

func normalizeSchemaName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var builder strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		builder.WriteString(string(runes))
	}
	return builder.String()
}

// makeCollisionSafeNamesFromComponents creates collision safe names for the components.
// This needs to be run before any routes or components are generated.
// As the makeComponentRefName function relies on the collisionSafeNames map.
func makeCollisionSafeNamesFromComponents(components []astra.Field) {
	type componentNameEntry struct {
		keys           []string
		baseName       string
		normalizedName string
		pkg            string
	}

	collisionSafeNames = make(map[string]string)
	entries := make([]componentNameEntry, 0)

	for _, component := range components {
		bindingTags, uniqueBindings := astra.ExtractBindingTags(component.StructFields)
		if uniqueBindings {
			for _, bindingType := range bindingTags {
				name := component.Name
				if bindingType != astTraversal.NoBindingTag {
					name = component.Name + "_" + string(bindingType)
				}
				entries = append(entries, componentNameEntry{
					keys:           []string{collisionSafeKey(bindingType, component.Name, component.Package)},
					baseName:       name,
					normalizedName: normalizeSchemaName(name),
					pkg:            component.Package,
				})
			}
			continue
		}

		keys := make([]string, 0, len(bindingTags))
		for _, bindingType := range bindingTags {
			keys = append(keys, collisionSafeKey(bindingType, component.Name, component.Package))
		}
		entries = append(entries, componentNameEntry{
			keys:           keys,
			baseName:       component.Name,
			normalizedName: normalizeSchemaName(component.Name),
			pkg:            component.Package,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].normalizedName == entries[j].normalizedName {
			if entries[i].pkg == entries[j].pkg {
				return strings.Join(entries[i].keys, ".") < strings.Join(entries[j].keys, ".")
			}
			return entries[i].pkg < entries[j].pkg
		}
		return entries[i].normalizedName < entries[j].normalizedName
	})

	counts := make(map[string]int)
	for _, entry := range entries {
		counts[entry.normalizedName]++
	}

	used := make(map[string]int)
	for _, entry := range entries {
		finalName := entry.baseName
		if counts[entry.normalizedName] > 1 {
			used[entry.normalizedName]++
			finalName = fmt.Sprintf("%s_%d", entry.baseName, used[entry.normalizedName])
		}
		for _, key := range entry.keys {
			collisionSafeNames[key] = finalName
		}
	}
}

// makeComponentRef creates a reference to the component in the OpenAPI specification.
func makeComponentRef(bindingType astTraversal.BindingTagType, name, pkg string) (string, bool) {
	componentName, bound := makeComponentRefName(bindingType, name, pkg)
	if !bound {
		return "", bound
	}

	return "#/components/schemas/" + componentName, bound
}

// makeComponentRefName converts the component and package name to a valid OpenAPI reference name (to avoid collisions).
func makeComponentRefName(bindingType astTraversal.BindingTagType, name, pkg string) (string, bool) {
	componentName, bound := collisionSafeNames[collisionSafeKey(bindingType, name, pkg)]
	if !bound {
		componentName, bound = collisionSafeNames[collisionSafeKey(astTraversal.NoBindingTag, name, pkg)]
	}

	return componentName, bound
}

func overrideFieldSchema(bindingType astTraversal.BindingTagType, component astra.Field, field astra.Field, fieldBinding astTraversal.BindingTag) (Schema, bool) {
	if getPackageName(component.Package) == "proto" && component.Name == "Blog" && fieldBinding.Name == "sharedThread" && field.Type == "ChatThread" {
		componentRef, bound := makeComponentRef(bindingType, "SimpleChatThread", field.Package)
		if bound {
			return Schema{Ref: componentRef}, true
		}
	}

	return Schema{}, false
}

// componentToSchema converts a component to a schema.
func componentToSchema(service *astra.Service, component astra.Field, bindingType astTraversal.BindingTagType) (schema Schema, bound bool) {
	if _, ok := service.GetTypeMapping(component.Name, component.Package); ok {
		return mapTypeFormat(service, component.Name, component.Package), true
	}

	if component.Type == "struct" {
		embeddedProperties := make([]Schema, 0)
		schema = Schema{
			Type:       "object",
			Properties: make(map[string]Schema),
		}
		for _, field := range component.StructFields {
			// We should aim to use doc comments in the future.
			// However https://github.com/OAI/OpenAPI-Specification/issues/1514.
			if field.IsEmbedded {
				componentRef, componentBound := makeComponentRef(bindingType, field.Type, field.Package)
				if componentBound {
					embeddedProperties = append(embeddedProperties, Schema{
						Ref: componentRef,
					})
				}

				continue
			}

			fieldBinding := field.StructFieldBindingTags[bindingType]
			fieldNoBinding := field.StructFieldBindingTags[astTraversal.NoBindingTag]
			if fieldBinding == (astTraversal.BindingTag{}) && fieldNoBinding == (astTraversal.BindingTag{}) {
				return Schema{}, false
			}
			if fieldBinding == (astTraversal.BindingTag{}) {
				fieldBinding = fieldNoBinding
			}

			if !fieldBinding.NotShown {
				if override, ok := overrideFieldSchema(bindingType, component, field, fieldBinding); ok {
					schema.Properties[fieldBinding.Name] = override
					continue
				}

				fieldSchema, fieldBound := componentToSchema(service, field, bindingType)
				if fieldBound {
					schema.Properties[fieldBinding.Name] = fieldSchema
				}
			}
		}

		if len(embeddedProperties) > 0 {
			if len(schema.Properties) == 0 {
				schema.AllOf = embeddedProperties
			} else {
				schema.AllOf = append(embeddedProperties, Schema{
					Properties: schema.Properties,
				})

				schema.Properties = nil
			}
		}
	} else if component.Type == "slice" {
		itemSchema := mapPredefinedTypeFormat(component.SliceType)

		if itemSchema.Type == "" && !astra.IsAcceptedType(component.SliceType) {
			componentRef, componentBound := makeComponentRef(bindingType, component.SliceType, component.Package)
			if componentBound {
				itemSchema = Schema{
					Ref: componentRef,
				}
			}
		}

		schema = Schema{
			Type:  "array",
			Items: &itemSchema,
		}
	} else if component.Type == "array" {
		itemSchema := mapPredefinedTypeFormat(component.ArrayType)

		if itemSchema.Type == "" && !astra.IsAcceptedType(component.ArrayType) {
			componentRef, componentBound := makeComponentRef(bindingType, component.ArrayType, component.Package)
			if componentBound {
				itemSchema = Schema{
					Ref: componentRef,
				}
			}
		}

		schema = Schema{
			Type:      "array",
			Items:     &itemSchema,
			MaxLength: int(component.ArrayLength),
		}
	} else if component.Type == "map" {
		additionalProperties := mapMapValueSchema(bindingType, component)
		schema = Schema{
			Type:                 "object",
			AdditionalProperties: &additionalProperties,
		}
	} else {
		schema = mapPredefinedTypeFormat(component.Type)
		if schema.Type == "" && !astra.IsAcceptedType(component.Type) {
			componentRef, componentBound := makeComponentRef(bindingType, component.Type, component.Package)
			if componentBound {
				schema = Schema{
					Ref: componentRef,
				}
			}
		} else {
			if len(component.EnumValues) > 0 {
				schema.Enum = component.EnumValues
				if len(component.EnumNames) == len(component.EnumValues) {
					hasName := false
					for _, name := range component.EnumNames {
						if name != "" {
							hasName = true
							break
						}
					}
					if hasName {
						schema.XEnumVarNames = component.EnumNames
					}
				}
			}
		}
	}

	return schema, true
}
