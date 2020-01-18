package hydrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jinzhu/gorm"

	_ "github.com/go-sql-driver/mysql"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	retCode := setup(m)
	os.Exit(retCode)
}

func setup(m *testing.M) int {
	os.Setenv("TZ", "UST")

	username := os.Getenv("TEST_DB_USERNAME")
	if username == "" {
		username = "root"
	}
	password := os.Getenv("TEST_DB_PASSWORD")
	if password == "" {
		password = "password"
	}
	host := os.Getenv("TEST_DB_HOST")
	if host == "" {
		host = "mysql"
	}

	database := "HYDRATE"

	database = fmt.Sprintf("TEST_%s_%d", database, time.Now().UnixNano()/1000)

	drop := createDB(username, password, host, database)
	defer drop()

	url := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&loc=Local", username, password, host, database)
	var err error
	testDB, err = gorm.Open("mysql", url)
	if err != nil {
		panic(err)
	}
	defer testDB.Close()

	db := testDB.Exec(`CREATE TABLE authors (
	author_id int(11) unsigned NOT NULL AUTO_INCREMENT,
	name varchar(128) NOT NULL,
	created_at timestamp NOT NULL,
	PRIMARY KEY (author_id)
	)`)

	db = db.Exec(`CREATE TABLE isbns (
	isbn_id int(11) unsigned NOT NULL AUTO_INCREMENT,
	textbook_id int(11) unsigned NOT NULL,
	isbn varchar(128) NOT NULL,
	created_at timestamp NOT NULL,
	PRIMARY KEY (isbn_id),
	KEY textbook_id (textbook_id)
	)`)

	db = db.Exec(`CREATE TABLE textbooks (
	textbook_id int(10) unsigned NOT NULL AUTO_INCREMENT,
	author_id int(11) unsigned NULL,
	name varchar(64) COLLATE utf8_unicode_ci NOT NULL,
		created_at timestamp NOT NULL,
	PRIMARY KEY (textbook_id),
	KEY author_id (author_id)
	)`)
	db = db.Exec(`CREATE TABLE sections (
	section_id int(11) unsigned NOT NULL AUTO_INCREMENT,
	textbook_id int(10) unsigned NOT NULL,
	title varchar(128) COLLATE utf8_unicode_ci NOT NULL,
	created_at timestamp NOT NULL,
	PRIMARY KEY (section_id),
	KEY textbook_id (textbook_id)
	)`)

	db = db.Exec(`CREATE TABLE exercises (
	exercise_id int(11) unsigned NOT NULL AUTO_INCREMENT,
	section_id int(10) unsigned NOT NULL,
	title varchar(128) COLLATE utf8_unicode_ci NOT NULL,
	ordering int(11) unsigned DEFAULT 0,
	created_at timestamp NOT NULL,
	PRIMARY KEY (exercise_id),
	KEY section_id (section_id)
	)`)

	db = db.Exec(`CREATE TABLE solutions (
	solution_id int(11) unsigned NOT NULL AUTO_INCREMENT,
	exercise_id int(10) unsigned NOT NULL,
	text varchar(128) COLLATE utf8_unicode_ci NOT NULL,
	PRIMARY KEY (solution_id),
	KEY exercise_id (exercise_id)
	)`)

	db = db.Exec(`
	INSERT INTO authors (author_id, name, created_at)
	VALUES
	(1, "a1", "2019-01-01"),
	(2, "a2", "2018-01-01")`)

	db = db.Exec(`
	INSERT INTO textbooks (textbook_id, author_id, name, created_at)
	VALUES
	(1, null, "t1", "2019-01-01"),
	(2, 1, "t2", "2018-01-01")`)

	db = db.Exec(`
	INSERT INTO isbns (isbn_id, textbook_id, isbn, created_at)
	VALUES
	(1, 1, "i1", "2019-01-01"),
	(2, 1, "i2", "2018-01-01"),
	(3, 2, "i3", "2018-01-01")`)

	db = db.Exec(`
	INSERT INTO sections (section_id, textbook_id, title, created_at)
	VALUES
	(1, 1, "t1-s1", "2019-02-01"),
	(2, 1, "t1-s2", "2019-03-01"),
	(3, 1, "t1-s3", "2019-03-01")`)

	db = db.Exec(`
	INSERT INTO exercises (exercise_id, section_id, title, ordering, created_at)
	VALUES
	(1, 1, "t1-s1-e1", 1, "2019-04-01"),
	(2, 1, "t1-s1-e2", 2, "2019-04-01"),
	(3, 3, "t1-s3-e1", 1, "2019-04-01"),
	(4, 3, "t1-s3-e2", 2, "2019-04-01")`)

	if db.Error != nil {
		panic(db.Error)
	}

	return m.Run()
}

func createDB(username, password, host, database string) func() {
	i := 0
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s)/", username, password, host))
	if err != nil {
		panic(err)
	}

	for i <= 20 {
		err = db.Ping()
		if err != nil {
			if i < 20 {
				time.Sleep(1 * time.Second)
				i++
				continue
			} else {
				panic(err)
			}
		}
		break
	}

	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE `%s`", database))
	if err != nil {
		panic(fmt.Errorf("failed to create database: %w", err))
	}

	return func() {
		_, err = db.Exec(fmt.Sprintf("DROP DATABASE `%s`", database))
		if err != nil {
			panic(fmt.Errorf("failed to drop database %w", err))
		}
		db.Close()
	}
}

func TestQuery(t *testing.T) {
	type runner interface {
		Run(context.Context, ...interface{}) error
	}
	tests := []struct {
		name   string
		runner runner
		wantErr bool
	}{
		{
			name: "should load from single query",
			runner: NewQuery(testDB, `FROM textbooks t
		LEFT JOIN isbns i ON i.textbook_id = t.textbook_id
	   LEFT JOIN sections s on t.textbook_id = s.textbook_id
		LEFT JOIN exercises e ON e.section_id = s.section_id
		LEFT JOIN authors a ON a.author_id = t.author_id
	   WHERE t.textbook_id in (?, ?)
		ORDER BY t.textbook_id, i.isbn_id, s.section_id, e.exercise_id, a.author_id`, 1, 2).
				AddModel(Textbook{}, "t").
				AddModel(Isbn{}, "i").
				AddModel(Section{}, "s").
				AddModel(Exercise{}, "e").
				AddModel(Author{}, "a"),
		},
		{
			name: "should load from multi query",
			runner: MultiQuery{
				NewQuery(testDB, `FROM textbooks t
	   LEFT JOIN sections s on t.textbook_id = s.textbook_id
		LEFT JOIN exercises e ON e.section_id = s.section_id
	   WHERE t.textbook_id in (?, ?)
		ORDER BY t.textbook_id, s.section_id, e.exercise_id`, 1, 2).
					AddModel(Textbook{}, "t").
					AddModel(Section{}, "s").
					AddModel(Exercise{}, "e"),

				NewQuery(testDB, `FROM textbooks t
		JOIN authors a ON a.author_id = t.author_id
		WHERE t.textbook_id in (?, ?)
		ORDER BY a.author_id`, 1, 2).
					AddModel(&Author{}, "a"),

				NewQuery(testDB, `FROM textbooks t
		JOIN isbns i ON i.textbook_id = t.textbook_id
		WHERE t.textbook_id in (?, ?)
		ORDER BY i.isbn_id`, 1, 2).
					AddModel(&Isbn{}, "i"),
			},
		},
		{
			name: "should use independent aliases in multi query",
			runner: MultiQuery{
				NewQuery(testDB, `FROM textbooks tb
	   WHERE tb.textbook_id in (?)`, 1).
					AddModel(&Textbook{}, "tb"),

				NewQuery(testDB, `FROM textbooks t
	   WHERE t.textbook_id in (?)`, 2).
					AddModel(&Textbook{}, "t"),
			},
		},
		{
			name: "should use table name as alias if not provided",
			runner: NewQuery(testDB, `FROM textbooks
	   WHERE textbook_id in (?)`, 1).AddModel(&Textbook{}, ""),
		},
		{
	   		name: "should error if query has error",
			runner: NewQuery(testDB, `FROM textbooks
	   WHERE not_a_column in (?)`, 1).AddModel(&Textbook{}, ""),
	   		wantErr: true,
		},
		{
			name: "should error if one multi query has error",
			runner: MultiQuery{
				NewQuery(testDB, `FROM textbooks tb
	   WHERE tb.textbook_id in (?)`, 1).
					AddModel(&Textbook{}, "tb"),
				NewQuery(testDB, `FROM textbooks
	   WHERE not_a_column in (?)`, 1).AddModel(&Textbook{}, ""),
			},
			wantErr: true,
		},
		{
			name: "should error if struct can not scan into result",
			runner: NewQuery(testDB, `FROM textbooks t
	   WHERE textbook_id in (?)`, 1).AddModel(BadTextbook{}, "t"),
	   wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var textbooks []*Textbook //test slice array output
			var authors []Author      //test slice struct output
			var isbn *Isbn            //test ptr output
			var exercise Exercise     //test struct output
			err := tt.runner.Run(context.Background(), &textbooks, &authors, &isbn, &exercise)
			if err != nil {
				if !tt.wantErr {
					t.Error(err)
				}
				return
			}

			if tt.wantErr {
				t.Error(fmt.Errorf("expected error result from unassignable output variable"))
				return
			}

			g := golden{Name: tt.name}
			data, err := json.Marshal(map[string]interface{}{
				"Textbooks": textbooks,
				"Authors":   authors,
				"Isbn":      isbn,
				"Exercise":  exercise,
			})

			if err != nil {
				t.Error(err)
				return
			}
			g.Equal(t, data)
		})
	}

	t.Run("should error if output can not be assigned", func(t *testing.T) {
		var tb Textbook
		err := NewQuery(testDB, `FROM textbooks t
	  WHERE textbook_id in (?)`, 1).AddModel(&Textbook{}, "t").Run(context.Background(), tb)

		if err == nil {
			t.Error(fmt.Errorf("expected error result from unassignable output variable"))
		}
	})
}

/**
goos: darwin
goarch: amd64
pkg: github.com/coursehero/hydrate
BenchmarkHydrate/S5:E10:I3/Preload-12 	     			 228	   5264193 ns/op	  170172 B/op	    3283 allocs/op
BenchmarkHydrate/S5:E10:I3/Query-12   	     			 408	   2973970 ns/op	  163543 B/op	    8394 allocs/op
BenchmarkHydrate/S5:E10:I3/MultiQuery-12         	     409	   2847169 ns/op	   72650 B/op	    2479 allocs/op
BenchmarkHydrate/S2000:E2:I2/Preload-12          	      22	  46077057 ns/op	16378600 B/op	  320521 allocs/op
BenchmarkHydrate/S2000:E2:I2/Query-12            	      14	  76894843 ns/op	 9535165 B/op	  497835 allocs/op
BenchmarkHydrate/S2000:E2:I2/MultiQuery-12       	      36	  32554055 ns/op	 4807059 B/op	  204267 allocs/op
BenchmarkHydrate/S10:E10:I10/Preload-12          	     205	   5807946 ns/op	  319066 B/op	    6551 allocs/op
BenchmarkHydrate/S10:E10:I10/Query-12            	     122	   9622931 ns/op	  903677 B/op	   53810 allocs/op
BenchmarkHydrate/S10:E10:I10/MultiQuery-12       	     358	   3222708 ns/op	  126218 B/op	    4857 allocs/op
*/
func BenchmarkHydrate(b *testing.B) {
	configs := []struct {
		sections  int
		exercises int
		isbns     int
	}{
		{
			sections:  5,
			exercises: 10,
			isbns:     3,
		},
		{
			sections:  2000,
			exercises: 2,
			isbns:     2,
		},
		{
			sections:  10,
			exercises: 10,
			isbns:     10,
		},
	}

	benchmarks := []struct {
		name   string
		runner func(uint) error
	}{
		{
			name: "Preload",
			runner: func(textbookID uint) error {
				return testDB.Preload("Sections.Exercises").Preload("Isbns").Find(&Textbook{TextbookID: textbookID}).Error
			},
		},
		{
			name: "Query",
			runner: func(textbookID uint) error {
				return NewQuery(testDB, `FROM textbooks t
	LEFT JOIN isbns i ON i.textbook_id = t.textbook_id
   LEFT JOIN sections s on t.textbook_id = s.textbook_id
	LEFT JOIN exercises e ON e.section_id = s.section_id
	LEFT JOIN solutions so ON so.exercise_id = e.exercise_id
	LEFT JOIN authors a ON a.author_id = t.author_id
   WHERE t.textbook_id in (?)
	ORDER BY t.textbook_id, i.isbn_id, s.section_id, e.exercise_id, a.author_id`, textbookID).
					AddModel(Textbook{}, "t").
					AddModel(Isbn{}, "i").
					AddModel(Section{}, "s").
					AddModel(Exercise{}, "e").
					AddModel(Author{}, "a").
					Run(context.Background(), &Textbook{})
			},
		},
		{
			name: "MultiQuery",
			runner: func(textbookID uint) error {
				return MultiQuery{
					NewQuery(testDB, `FROM sections s
		LEFT JOIN exercises e ON e.section_id = s.section_id
		WHERE s.textbook_id in (?)
		ORDER BY s.section_id, e.exercise_id`, textbookID).
						AddModel(Section{}, "s").
						AddModel(Exercise{}, "e"),

					NewQuery(testDB, `FROM textbooks t
		JOIN isbns i ON i.textbook_id = t.textbook_id
		JOIN authors a ON a.author_id = t.author_id
		WHERE t.textbook_id in (?)
		ORDER BY i.isbn_id`, textbookID).
						AddModel(Textbook{}, "t").
						AddModel(Author{}, "a").
						AddModel(Isbn{}, "i"),
				}.Run(context.Background(), &Textbook{})
			},
		},
	}
	for _, c := range configs {
		textbookID := generateHierarchy(testDB, c.sections, c.exercises, c.isbns)

		b.Run(fmt.Sprintf("S%d:E%d:I%d", c.sections, c.exercises, c.isbns), func(b *testing.B) {
			for _, bm := range benchmarks {
				b.Run(bm.name, func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						err := bm.runner(textbookID)
						if err != nil {
							b.Error(err)
							return
						}
					}
				})
			}
		})
	}
}

func generateHierarchy(db *gorm.DB, sections, exercises, isbns int) uint {
	//var author Author
	//db.Save(&author)

	var textbook Textbook
	//textbook.AuthorID = sql.NullInt64{Int64: int64(author.AuthorID), Valid: true}
	db.Save(&textbook)

	for s := 0; s < sections; s++ {
		var section Section
		section.TextbookID = textbook.TextbookID
		db.Save(&section)
		for e := 0; e < exercises; e++ {
			var exercise Exercise
			exercise.SectionID = *section.SectionID
			db.Save(&exercise)
		}
	}

	for i := 0; i < isbns; i++ {
		var isbn Isbn
		isbn.TextbookID = sql.NullInt64{Int64: int64(textbook.TextbookID), Valid: true}
		db.Save(&isbn)
	}

	return textbook.TextbookID
}

type Author struct {
	AuthorID uint `gorm:"primary_key"`
	Name     string
}

type Textbook struct {
	TextbookID uint `gorm:"primary_key"`
	AuthorID   sql.NullInt64
	Name       string
	CreatedAt  *time.Time

	AuthorVal Author  `gorm:"foreignkey:AuthorID;association_foreignkey:AuthorID"`
	AuthorPtr *Author `gorm:"foreignkey:AuthorID;association_foreignkey:AuthorID"`

	Isbns    []Isbn     `gorm:"foreignkey:TextbookID;association_foreignkey:TextbookID"` //tests struct slice
	Sections []*Section `gorm:"foreignkey:TextbookID;association_foreignkey:TextbookID"` //tests pointer slice
}

//Bad Textbook that won't scan into our table
type BadTextbook struct {
	TextbookID uint `gorm:"primary_key"`
	Name       sql.NullInt64
}

type Isbn struct {
	IsbnID     uint `gorm:"primary_key"`
	TextbookID sql.NullInt64
	Isbn       string
}

type Section struct {
	SectionID  *uint `gorm:"primary_key"`
	TextbookID uint
	Title      string
	CreatedAt  *time.Time

	Exercises []Exercise `gorm:"foreignkey:SectionID"` //tests non pointer slice
}

type Exercise struct {
	ExerciseID uint `gorm:"primary_key"`
	SectionID  uint
	Ordering   uint
	Title      string
	CreatedAt  *time.Time
}
