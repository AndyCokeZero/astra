package openapi

import (
	"github.com/ls6-events/astra"
	"github.com/ls6-events/astra/astTraversal"
)

func mapParamToSchema(bindingType astTraversal.BindingTagType, param astra.Param) (Schema, bool) {
	if param.IsBound {
		return mapFieldToSchema(bindingType, param.Field)
	} else if param.IsArray {
		itemSchema := mapPredefinedTypeFormat(param.Field.Type)
		if !astra.IsAcceptedType(param.Field.Type) {
			componentRef, bound := makeComponentRef(bindingType, param.Field.Type, param.Field.Package)
			if bound {
				itemSchema = Schema{
					Ref: componentRef,
				}
			}
		}
		return Schema{
			Type:  "array",
			Items: &itemSchema,
		}, true
	} else if param.IsMap {
		var additionalProperties Schema
		if !astra.IsAcceptedType(param.Field.Type) {
			componentRef, bound := makeComponentRef(bindingType, param.Field.Type, param.Field.Package)
			if bound {
				additionalProperties.Ref = componentRef
			}
		} else {
			additionalProperties = mapPredefinedTypeFormat(param.Field.Type)
		}
		return Schema{
			Type:                 "object",
			AdditionalProperties: &additionalProperties,
		}, true
	} else {
		return mapPredefinedTypeFormat(param.Field.Type), true
	}
}

func mapFieldToSchema(bindingType astTraversal.BindingTagType, field astra.Field) (Schema, bool) {
	if field.Type == "struct" && len(field.StructFields) > 0 {
		if schema, ok := mapInlineStructToSchema(bindingType, field); ok {
			return schema, true
		}
	}
	if !astra.IsAcceptedType(field.Type) {
		componentRef, bound := makeComponentRef(bindingType, field.Type, field.Package)
		if bound {
			return Schema{
				Ref: componentRef,
			}, true
		}

		return Schema{}, false
	} else {
		schema := mapPredefinedTypeFormat(field.Type)
		if field.Type == "slice" {
			itemSchema := Schema{
				Type: mapPredefinedTypeFormat(field.SliceType).Type,
			}
			if !astra.IsAcceptedType(field.SliceType) {
				componentRef, bound := makeComponentRef(bindingType, field.SliceType, field.Package)
				if bound {
					itemSchema = Schema{
						Ref: componentRef,
					}
				}
			}
			schema.Items = &itemSchema
		} else if field.Type == "map" {
			additionalProperties := mapMapValueSchema(bindingType, field)
			schema.AdditionalProperties = &additionalProperties
		}

		return schema, true
	}
}

func mapInlineStructToSchema(bindingType astTraversal.BindingTagType, field astra.Field) (Schema, bool) {
	embeddedProperties := make([]Schema, 0)
	schema := Schema{
		Type:       "object",
		Properties: make(map[string]Schema),
	}

	for _, structField := range field.StructFields {
		if structField.IsEmbedded {
			componentRef, componentBound := makeComponentRef(bindingType, structField.Type, structField.Package)
			if componentBound {
				embeddedProperties = append(embeddedProperties, Schema{
					Ref: componentRef,
				})
			}
			continue
		}

		fieldBinding := structField.StructFieldBindingTags[bindingType]
		fieldNoBinding := structField.StructFieldBindingTags[astTraversal.NoBindingTag]
		if fieldBinding == (astTraversal.BindingTag{}) && fieldNoBinding == (astTraversal.BindingTag{}) {
			return Schema{}, false
		}
		if fieldBinding == (astTraversal.BindingTag{}) {
			fieldBinding = fieldNoBinding
		}

		if !fieldBinding.NotShown {
			fieldSchema, fieldBound := mapFieldToSchema(bindingType, structField)
			if fieldBound {
				schema.Properties[fieldBinding.Name] = ensureSchema(fieldSchema)
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

	return schema, true
}

func ensureSchema(schema Schema) Schema {
	if isSchemaEmpty(schema) {
		return Schema{Type: "string"}
	}
	return schema
}

func isSchemaEmpty(schema Schema) bool {
	return schema.Ref == "" &&
		schema.Type == "" &&
		schema.Items == nil &&
		schema.AdditionalProperties == nil &&
		schema.Not == nil &&
		len(schema.Enum) == 0 &&
		len(schema.Required) == 0 &&
		len(schema.AllOf) == 0 &&
		len(schema.OneOf) == 0 &&
		len(schema.AnyOf) == 0 &&
		len(schema.Properties) == 0
}

func mapMapValueSchema(bindingType astTraversal.BindingTagType, field astra.Field) Schema {
	switch field.MapValueType {
	case "slice":
		itemSchema := mapPredefinedTypeFormat(field.MapValueSliceType)
		if itemSchema.Type == "" && !astra.IsAcceptedType(field.MapValueSliceType) {
			pkg := field.MapValuePackage
			if pkg == "" {
				pkg = field.Package
			}
			if componentRef, bound := makeComponentRef(bindingType, field.MapValueSliceType, pkg); bound {
				itemSchema = Schema{Ref: componentRef}
			}
		}
		return Schema{
			Type:  "array",
			Items: &itemSchema,
		}
	case "array":
		itemSchema := mapPredefinedTypeFormat(field.MapValueArrayType)
		if itemSchema.Type == "" && !astra.IsAcceptedType(field.MapValueArrayType) {
			pkg := field.MapValuePackage
			if pkg == "" {
				pkg = field.Package
			}
			if componentRef, bound := makeComponentRef(bindingType, field.MapValueArrayType, pkg); bound {
				itemSchema = Schema{Ref: componentRef}
			}
		}
		return Schema{
			Type:  "array",
			Items: &itemSchema,
		}
	default:
		additionalProperties := mapPredefinedTypeFormat(field.MapValueType)
		if additionalProperties.Type == "" && !astra.IsAcceptedType(field.MapValueType) {
			pkg := field.MapValuePackage
			if pkg == "" {
				pkg = field.Package
			}
			if componentRef, bound := makeComponentRef(bindingType, field.MapValueType, pkg); bound {
				additionalProperties.Ref = componentRef
			}
		}
		return additionalProperties
	}
}

// mapTypeFormat maps the type with the list of types from the service.
// This should be primarily used for custom types in components that need to be mapped.
func mapTypeFormat(service *astra.Service, acceptedType string, pkg string) Schema {
	if acceptedType, ok := service.GetTypeMapping(acceptedType, pkg); ok {
		if acceptedType.Type == "" {
			return Schema{}
		}
		return Schema{
			Type:   acceptedType.Type,
			Format: acceptedType.Format,
		}
	}

	return Schema{}
}

// mapPredefinedTypeFormat maps the type with the list of types that are predefined.
// This should be primarily used for types that are not custom types, i.e. everywhere except top level components.
func mapPredefinedTypeFormat(acceptedType string) Schema {
	if acceptedType, ok := astra.PredefinedTypeMap[acceptedType]; ok {
		if acceptedType.Type == "" {
			return Schema{}
		}
		return Schema{
			Type:   acceptedType.Type,
			Format: acceptedType.Format,
		}
	}

	return Schema{}
}

// getQueryParamStyle returns the style of the query parameter, based on the schema.
func getQueryParamStyle(schema Schema) (style string, explode bool) {
	if schema.Type == "object" {
		return "deepObject", true
	}

	// The default behavior is to use the form style.
	// For arrays, we want comma separated values.
	return "form", false
}

// findComponentByPackageAndType finds the schema by the package and type.
func findComponentByPackageAndType(fields []astra.Field, pkg string, typeName string) (astra.Field, bool) {
	for _, field := range fields {
		if field.Package == pkg && field.Name == typeName {
			return field, true
		}
	}

	return astra.Field{}, false
}
