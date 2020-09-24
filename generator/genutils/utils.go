package genutils

import (
	"strings"

	"github.com/iancoleman/strcase"
	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-tools/pkg/crd"
	"sigs.k8s.io/controller-tools/pkg/markers"
)

const (
	patchStrategyTagKey = "patchStrategy"
	patchMergeKeyTagKey = "patchMergeKey"
	// MergePatchStrategy is the name of the Merge patch strategy
	MergePatchStrategy = "merge"
	// ReplacePatchStrategy is the name of the Replace patch strategy
	ReplacePatchStrategy = "replace"
)

// ContainsPatchStrategy reads the field tags to check whether the given patch strategy is defined
func ContainsPatchStrategy(field *markers.FieldInfo, strategy string) bool {
	patchStrategy := field.Tag.Get(patchStrategyTagKey)
	if patchStrategy == "" {
		return false
	}

	for _, s := range strings.Split(patchStrategy, ",") {
		if s == strategy {
			return true
		}
	}
	return false
}

// GetPatchMergeKey reads the field tags to retrieve the patch merge key. It returns nil if no such key is defined
func GetPatchMergeKey(field *markers.FieldInfo) string {
	return field.Tag.Get(patchMergeKeyTagKey)
}

// Visitor is the type of a function that visits one level of Json schema
type Visitor func(schema *apiext.JSONSchemaProps) (newVisitor Visitor, stop bool)

type visitorStruct struct {
	VisitFunc Visitor
}

func (v visitorStruct) Visit(schema *apiext.JSONSchemaProps) crd.SchemaVisitor {
	newVisitor, stop := v.VisitFunc(schema)
	if stop {
		return nil
	}

	if newVisitor == nil {
		return v
	}
	return visitorStruct{newVisitor}
}

// EditJSONSchema allows editing a Json Schema by applying the visitor function to each level of the schema.
func EditJSONSchema(jsonSchema *apiext.JSONSchemaProps, visitor Visitor) {
	crd.EditSchema(jsonSchema, visitorStruct{visitor})
}

// AddUnionOneOfConstraints adds oneOf constraints in the given Json schema for all the unions, based on the authorized values of the union members
func AddUnionOneOfConstraints(jsonSchema *apiext.JSONSchemaProps, unionDiscriminators []markers.FieldInfo, removeDiscriminators bool, fieldsToSkip ...string) {
	mainVisitor := func(schema *apiext.JSONSchemaProps) (newVisitor Visitor, stop bool) {
		if schema == nil {
			return
		}
		if schema.Type != "object" {
			return
		}

		if len(schema.Properties) > 0 {
		discriminatorLoop:
			for _, discriminator := range unionDiscriminators {
				discriminatorPropertyName := strcase.ToLowerCamel(discriminator.Name)
				if discriminatorProp, found := schema.Properties[discriminatorPropertyName]; found &&
					discriminatorProp.Enum != nil &&
					len(discriminatorProp.Enum) != 0 {
					oneOf := []apiext.JSONSchemaProps{}
					newEnumValues := []apiext.JSON{}
				enumValueLoop:
					for _, enumValue := range discriminatorProp.Enum {
						fieldName := strings.Trim(string(enumValue.Raw), `"`)
						unionMemberProperty := strcase.ToLowerCamel(fieldName)
						if _, foundUnionMember := schema.Properties[unionMemberProperty]; !foundUnionMember {
							continue discriminatorLoop
						}
						for _, toSkip := range fieldsToSkip {
							if toSkip == fieldName {
								continue enumValueLoop
							}
						}
						newEnumValues = append(newEnumValues, enumValue)
						oneOf = append(oneOf, apiext.JSONSchemaProps{
							Required: []string{unionMemberProperty},
						})
					}
					schema.OneOf = oneOf
					discriminatorProp.Enum = newEnumValues

					if removeDiscriminators {
						delete(schema.Properties, discriminatorPropertyName)
					} else {
						schema.Properties[discriminatorPropertyName] = discriminatorProp
					}
					for _, toSkip := range fieldsToSkip {
						delete(schema.Properties, strcase.ToLowerCamel(toSkip))
					}
				}
			}
		}
		return
	}

	EditJSONSchema(jsonSchema, mainVisitor)
}

var (
	// UnionMarker is the definition of the union marker, as defined in https://github.com/kubernetes/enhancements/blob/master/keps/sig-api-machinery/20190325-unions.md#proposal
	UnionMarker = markers.Must(markers.MakeDefinition("union", markers.DescribesType, struct{}{}))
	// UnionDiscriminatorMarker is the definition of the union discriminator marker, as defined in https://github.com/kubernetes/enhancements/blob/master/keps/sig-api-machinery/20190325-unions.md#proposal
	UnionDiscriminatorMarker = markers.Must(markers.MakeDefinition("unionDiscriminator", markers.DescribesField, struct{}{}))
)

// RegisterUnionMarkers registers the `union` and `unionDiscriminator` markers
func RegisterUnionMarkers(into *markers.Registry) error {
	if err := markers.RegisterAll(into, UnionMarker, UnionDiscriminatorMarker); err != nil {
		return err
	}
	into.AddHelp(UnionMarker,
		markers.SimpleHelp("Devfile", "indicates that a given Struct type is a K8S union, and its fields (apart from the discriminator) are mutually exclusive. K8S unions are described here: https://github.com/kubernetes/enhancements/blob/master/keps/sig-api-machinery/20190325-unions.md#proposal"))
	into.AddHelp(UnionDiscriminatorMarker,
		markers.SimpleHelp("Devfile", "indicates that a given field of an union Struct type is the union discriminator. K8S unions are described here: https://github.com/kubernetes/enhancements/blob/master/keps/sig-api-machinery/20190325-unions.md#proposal"))
	return nil
}
