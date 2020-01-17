package hydrate

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/jinzhu/gorm"
)

//Query is used to define a query to hydrate data using a single query. Any number of models can be added to be loaded,
//each model will be added to the select query using the alias provided (or the table name if empty).
//Each model added will store unique items based on primary key values returned from the query. All relationships for
//each model will be filled by connecting to other loaded models using the gorm defined relationship.
type Query struct {
	db *gorm.DB

	query string
	args  []interface{}

	models []modelConfiguration
	//getModelLoader is used to return modelLoaders from model configuration. This is overwritten for MultiQueries
	//to allow modelLoaders to be shared across queries
	getModelLoader func([]modelConfiguration) ([]*modelLoader, []string)
}

//NewQuery will create a query with a given query and sql args
func NewQuery(db *gorm.DB, query string, args ...interface{}) Query {
	return Query{
		query:          query,
		args:           args,
		db:             db,
		getModelLoader: newModelLoaders,
	}
}

//AddModel will add a model to be loaded during execution. Its fields will be added to the select. If no alias
//is provided it will use the table name
func (r Query) AddModel(in interface{}, alias string) Query {
	r.models = append(r.models, modelConfiguration{in, alias})

	return r
}

//Run will run the query and put results in any outputs provided. Each output must be a pointer to a value that can be
//set.
//If a slice is provided it will fill with all results. If a single item is passed the first item will be returned. However
//no limiting will be done to the query.
func (r Query) Run(ctx context.Context, output ...interface{}) error {
	loaders, err := r.runQuery()
	if err != nil {
		return err
	}

	return fillOutput(loaders, output)
}

//runQuery will run the query and return modelLoaders with filled information. Relationships will not be filled
func (r Query) runQuery() ([]*modelLoader, error) {
	loaders, aliases := r.getModelLoader(r.models)
	selects := make([]string, 0, len(loaders))

	for i, l := range loaders {
		selects = append(selects, l.getSelectStatement(r.db, aliases[i]))
	}

	rows, err := r.db.Raw(fmt.Sprintf(`%s %s %s`, "SELECT", strings.Join(selects, ","), r.query), r.args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	scans := make([]interface{}, 0, len(loaders)*3)
	for _, l := range loaders {
		//s := l.scanValues
		//defer closer()
		scans = append(scans, l.scanValues...)
	}
	for rows.Next() {
		err := func() error {
			err := rows.Scan(scans...)
			if err != nil {
				return err
			}
			for _, l := range loaders {
				l.processScan()
			}
			return nil
		}()
		if err != nil {
			return nil, err
		}
	}

	return loaders, nil
}

//modelConfiguration is used to define how a model is used in a query.
type modelConfiguration struct {
	//example is an example of the models type
	example interface{}
	//alias is the alias to use within a query
	alias string
}

//newModelLoaders is the default implementation for getting modelLoaders from a model definition. This will create new
//loaders on each call.
func newModelLoaders(models []modelConfiguration) ([]*modelLoader, []string) {
	ret := make([]*modelLoader, 0, len(models))
	aliases := make([]string, 0, len(models))
	for _, m := range models {
		ret = append(ret, newModelLoader(m.example))
		aliases = append(aliases, m.alias)
	}
	return ret, aliases
}

//fillOutput will load output results from a slice of modelLoaders
func fillOutput(loaders []*modelLoader, output []interface{}) error {
	items := make(map[reflect.Type][]interface{}, len(loaders))
	for _, l := range loaders {
		items[baseType(l.itemType)] = l.items
	}

	for _, l := range loaders {
		err := l.finalize(items)
		if err != nil {
			return err
		}
	}

	for _, o := range output {
		val := reflect.ValueOf(o)

		if val.Kind() != reflect.Ptr || val.IsNil() || !val.Elem().CanSet() {
			return fmt.Errorf("type %s can not be set", val.Type())
		}

		t := baseType(val.Type())
		if items, ok := items[t]; ok {
			el := val.Elem()

			if el.Kind() == reflect.Slice {
				var addStruct bool
				if el.Type().Elem().Kind() != reflect.Ptr {
					//if the output expects a slice of structs not pointers, unwrap items as we add them
					addStruct = true
				}

				for _, i := range items {
					add := reflect.ValueOf(i)
					if addStruct {
						//unwrap pointer
						add = add.Elem()
					}

					el.Set(reflect.Append(el, add))
				}
			} else {
				if len(items) > 0 {
					add := reflect.ValueOf(items[0])
					if el.Kind() != reflect.Ptr {
						add = add.Elem()
					}
					el.Set(add)
					continue
				}
			}
		}
	}

	return nil
}

//MultiQuery will allow you to run multiple Queries and combine all results. Queries are run as normal but all
//structs are shared across all queries. Meaning relationships can be populated from independent queries, or even
//a collection of results from the same table can be loaded from multiple queries.
type MultiQuery []Query

//Run will run all queries and return output combined from all query runs.
func (m MultiQuery) Run(ctx context.Context, output ...interface{}) error {
	//define loaders we will share across each query run
	var loaders []*modelLoader
	loaderMap := make(map[reflect.Type]*modelLoader)

	//define a model loader we can use in the individual Queries to pull loaders from our shared map
	getModelLoader := func(models []modelConfiguration) ([]*modelLoader, []string) {
		ret := make([]*modelLoader, 0, len(models))
		aliases := make([]string, 0, len(models))
		for _, m := range models {
			modelType := baseType(reflect.TypeOf(m.example))
			ret = append(ret, loaderMap[modelType])
			aliases = append(aliases, m.alias)
		}
		return ret, aliases
	}

	for _, q := range m {
		for _, model := range q.models {
			modelType := baseType(reflect.TypeOf(model.example))
			if _, ok := loaderMap[modelType]; !ok {
				l := newModelLoader(model.example)
				loaderMap[modelType] = l
				loaders = append(loaders, l)
			}
		}
		q.getModelLoader = getModelLoader

		_, err := q.runQuery()
		if err != nil {
			return err
		}
	}

	return fillOutput(loaders, output)
}

//baseType will return the fully unwrapped type of slice
func baseType(t reflect.Type) reflect.Type {
	switch t.Kind() {
	case reflect.Array, reflect.Ptr, reflect.Slice:
		return baseType(t.Elem())
	}
	return t
}
