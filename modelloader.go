package hydrate

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"

	"github.com/jinzhu/gorm"
)

//modelLoader provides functionality for loading and storing a given model
type modelLoader struct {
	//itemType is the reflected type held by the modelLoader
	itemType reflect.Type
	//ms is the gorm.ModelStruct of the type being loaded
	ms *gorm.ModelStruct

	//selectFields holds a reference to all fields used when selecting
	selectFields []*gorm.StructField
	//keyFields holds all fields that represent the primary key of the model
	keyFields []*gorm.StructField
	//relationships holds all relationships that can be loaded in the model
	relationships []*gorm.StructField

	//items hold a flat list in order received of the structs loaded
	items []interface{}
	//storedKeys will record which primary keys are held in our item map
	storedKeys map[string]struct{}

	scanValues []interface{}
	fields     []reflect.Value
}

//newModelLoader will instantiate a new model loader with metadata loaded
func newModelLoader(in interface{}) *modelLoader {
	var ret modelLoader

	s := &gorm.Scope{Value: in}
	//get the gorm model struct so we can leverage gorm for DB and relationship information
	ms := s.GetModelStruct()

	//construct metadata fields we need from all struct fields
	for _, f := range ms.StructFields {
		if f.Relationship != nil {
			//track all relationships so we can map them later
			ret.relationships = append(ret.relationships, f)
		}

		if !f.IsNormal {
			//if it's not a relationship and not a normal field we don't care about it
			continue
		}

		if f.IsPrimaryKey {
			//track primary keys so we can track which items we've already processed
			ret.keyFields = append(ret.keyFields, f)
		}
		ret.selectFields = append(ret.selectFields, f)
	}

	ret.itemType = ms.ModelType
	ret.ms = ms
	ret.storedKeys = make(map[string]struct{})

	ret.scanValues = make([]interface{}, 0, len(ret.selectFields))
	ret.fields = make([]reflect.Value, 0, len(ret.selectFields))
	for _, f := range ret.selectFields {

		//we need to create a new value not tied to the struct that addresses the type given.
		//this allows the scan to always succeed and not fail if we get a null back for a non-nullable field
		newValue := reflect.New(reflect.PtrTo(f.Struct.Type))
		ret.fields = append(ret.fields, newValue)
		ret.scanValues = append(ret.scanValues, newValue.Interface())
	}

	return &ret
}

//getSelectStatement will return the select statement to use for this model
func (m modelLoader) getSelectStatement(db *gorm.DB, alias string) string {
	if alias == "" {
		alias = m.ms.TableName(db)
	}

	selects := make([]string, 0, len(m.selectFields))
	for _, f := range m.selectFields {
		selects = append(selects, fmt.Sprintf("%s.%s", alias, f.DBName))
	}

	return strings.Join(selects, ",")
}

func (m *modelLoader) processScan() {
	//build key string for identity map
	var keyVal strings.Builder
	for i, f := range m.selectFields {
		if !f.IsPrimaryKey {
			continue
		}
		val := m.fields[i].Elem()
		if val.IsNil() {
			//if a PK value is nil, we should be in a failed left join, skip row
			return
		}
		for val.Kind() == reflect.Ptr {
			val = val.Elem()
		}
		keyVal.WriteString(fmt.Sprintf("[%s:%v]", f.Name, val.Interface()))
	}
	key := keyVal.String()

	if _, ok := m.storedKeys[key]; ok {
		//we already have this PK, skip this row
		return
	}

	newItem := reflect.New(m.ms.ModelType).Elem()

	//go through each field we selected from
	for i, f := range m.selectFields {
		//get the value we pulled out of sql
		selectedValue := m.fields[i].Elem()

		if selectedValue.IsNil() {
			//nil primary key means we didn't load data for this row (this code does not support nullable primary keys)
			continue
		}

		//assign the sql value to our struct row
		val := newItem.FieldByName(f.Name)
		val.Set(selectedValue.Elem())
	}

	m.items = append(m.items, newItem.Addr().Interface())
	m.storedKeys[key] = struct{}{}
}

//finalize will finalize all items and load all available relationships
func (m *modelLoader) finalize(itemMap map[reflect.Type][]interface{}) error {
	//fill all relationships we can on our items
	for _, f := range m.relationships {
		items, ok := itemMap[baseType(f.Struct.Type)]
		if !ok {
			//this relationship isn't in our item map
			continue
		}

		lookup := make(map[string][]reflect.Value)

		//construct a map with possibilities of this relationship
		for _, n := range items {
			itemVal := reflect.ValueOf(n).Elem()

			//build a key for the attributes of this relationship
			var sb strings.Builder
			for i, name := range f.Relationship.ForeignFieldNames {
				val := itemVal.FieldByName(name).Interface()

				if valuer, ok := val.(driver.Valuer); ok {
					var err error
					val, err = valuer.Value()
					if err != nil {
						return err
					}
				}

				sb.WriteString(fmt.Sprintf("[%d:%v]", i, val))
			}

			key := sb.String()
			lookup[key] = append(lookup[key], itemVal.Addr())
		}

		//go through all models were tracking and fill in this relationship
		for _, item := range m.items {
			itemVal := reflect.ValueOf(item).Elem()
			relVal := itemVal.FieldByName(f.Name)

			//build a key for the attributes of this relationship
			var sb strings.Builder
			for i, name := range f.Relationship.AssociationForeignFieldNames {
				val := itemVal.FieldByName(name)
				if val.Kind() == reflect.Ptr && !val.IsNil() {
					val = val.Elem()
				}

				keyValue := val.Interface()
				if valuer, ok := keyValue.(driver.Valuer); ok {
					keyValue, _ = valuer.Value()
				}
				sb.WriteString(fmt.Sprintf("[%d:%v]", i, keyValue))
			}

			key := sb.String()
			//find items corresponding to this item for this relationship
			for _, newVal := range lookup[key] {
				//we have items to fill this relationship, fill it based on the struct
				if relVal.Kind() == reflect.Slice {
					//add the result to our slice
					if relVal.Type().Elem().Kind() != reflect.Ptr {
						//we have a slice of structs so add the struct we're pointing to
						newVal = newVal.Elem()
					}

					relVal.Set(reflect.Append(relVal, newVal))
				} else {
					//we don't have a slice so set the item to the first one we have and move on
					if relVal.Type().Kind() != reflect.Ptr {
						newVal = newVal.Elem()
					}

					relVal.Set(newVal)
					break
				}
			}
		}
	}

	return nil
}
