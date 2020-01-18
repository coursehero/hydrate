# Hydrate

[![Actions Status](https://github.com/coursehero/hydrate/workflows/build/badge.svg)](https://github.com/coursehero/hydrate/actions)
[![codecov](https://codecov.io/gh/coursehero/hydrate/branch/master/graph/badge.svg)](https://codecov.io/gh/coursehero/hydrate)

Hydrate is a package designed to work with [jinzhu/gorm](https://github.com/jinzhu/gorm) to provide an alternative to
Preload to load hierarchies efficiently. 

Preload will load each layer of a hiearchy as its own query by constructing a query
for each table using a `WHERE IN (?,?,...,?)` where the criteria includes the relationship keys to load. This can cause
hierarchies with large branching factor to generate queries with an extreme number of IDs in the `WHERE IN` clause
which can result in queries being significantly less performant.

With `hydrate` one or more queries can be constructed to load a full hierarchy of data.

## Usage

### Query

A query is constructed using `NewQuery` to pass in the database query used to load the full hierarchy. Models are added
using `AddModel` to tell `hydrate` which alias to use to load each model. `Run` takes in a context and any results you
want returned. Valid values for results are references to a struct, pointer to a struct, slice of structs, or slice of 
pointers of structs (ie. references to: `ModelType`, `*ModelType`, `[]ModelType`, or `[]*ModelType`). Multiple result
types can be passed and all will be loaded

```go
hydrate.NewQuery(db, `FROM textbooks t
        LEFT JOIN sections s on t.textbook_id = s.textbook_id
        LEFT JOIN exercises e ON e.section_id = s.section_id
        LEFT JOIN authors a ON a.author_id = t.author_id
        WHERE t.textbook_id in (?)
        ORDER BY t.textbook_id, s.section_id, a.author_id`, 1).
    AddModel(Textbook{}, "t").
    AddModel(Section{}, "s").
    AddModel(Exercise{}, "e").
    AddModel(Author{}, "a").
    Run(context.Background(), &textbooks)
```

### MultiQuery

MultiQuery is a type backed by `[]Query`. Multiple queries can be chained together to all load the same hierarchy. For
important code paths well crafted MultiQueries are typically how you will get the most performance. Separate sections of your
hierarchy can be loaded intelligently.

```go
hydrate.MultiQuery{
    hydrate.NewQuery(db, `FROM textbooks t
            LEFT JOIN sections s on t.textbook_id = s.textbook_id
            WHERE t.textbook_id in (?)
            ORDER BY t.textbook_id, s.section_id`, 1).
        AddModel(Textbook{}, "t").
        AddModel(Section{}, "s"),

    hydrate.NewQuery(db, `FROM textbooks t
            LEFT JOIN authors a ON a.author_id = t.author_id
            WHERE t.textbook_id in (?)
            ORDER BY t.textbook_id, s.section_id, a.author_id`, 1).
        AddModel(Author{}, "a"),
}.Run(context.Background(), &textbooks)
```

## Running Tests

Tests depend on a mysql database being available. The connection to this DB can be set with `TEST_DB_HOST`, 
`TEST_DB_USERNAME`, `TEST_DB_PASSWORD` environment variables.

Prior to executing tests a new schema will be created on the DB and that schema will be removed when tests complete.

```bash
$ TEST_DB_HOST=localhost TEST_DB_USERNAME=root TEST_DB_PASSWORD=password go test .
```

Tests use "golden" files which record the expected output of each test. This is typically json encoding of structs.
When updating or writing new tests the `-update` flag can be passed to update the golden files.

```bash
$ TEST_DB_HOST=localhost TEST_DB_USERNAME=root TEST_DB_PASSWORD=password go test . -update
```

### Benchmarks + Performance

Benchmarks will run actual queries against the test db, as a result timing can fluctuate. Performance will also vary 
drastically based on the shape of the hierarchy being loaded. Various configurations are run loading different amounts
of data and different relationship branching factors. Loading data all in a single query can be less performant than
gorm's Preload in some cases. However in most cases a well thought out MultiQuery tends to perform better.

An example of current benchmarks are below, which compare different configurations using standard `gorm`'s Preload, one large
`hydrate.Query`, and two queries using `hydrate.MultiQuery`:
```
goos: darwin
goarch: amd64
pkg: github.com/coursehero/hydrate
BenchmarkHydrate/S5:E10:I3/Preload-12 	     		     228	   5264193 ns/op	  170172 B/op	    3283 allocs/op
BenchmarkHydrate/S5:E10:I3/Query-12   	     		     408	   2973970 ns/op	  163543 B/op	    8394 allocs/op
BenchmarkHydrate/S5:E10:I3/MultiQuery-12         	     409	   2847169 ns/op	   72650 B/op	    2479 allocs/op
BenchmarkHydrate/S2000:E2:I2/Preload-12          	      22	  46077057 ns/op	16378600 B/op	  320521 allocs/op
BenchmarkHydrate/S2000:E2:I2/Query-12            	      14	  76894843 ns/op	 9535165 B/op	  497835 allocs/op
BenchmarkHydrate/S2000:E2:I2/MultiQuery-12       	      36	  32554055 ns/op	 4807059 B/op	  204267 allocs/op
BenchmarkHydrate/S10:E10:I10/Preload-12          	     205	   5807946 ns/op	  319066 B/op	    6551 allocs/op
BenchmarkHydrate/S10:E10:I10/Query-12            	     122	   9622931 ns/op	  903677 B/op	   53810 allocs/op
BenchmarkHydrate/S10:E10:I10/MultiQuery-12       	     358	   3222708 ns/op	  126218 B/op	    4857 allocs/op
```
