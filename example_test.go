package hydrate_test

import (
	"context"
	"fmt"

	"github.com/jinzhu/gorm"

	"github.com/coursehero/hydrate"
)

//A single query can be provided using hydrate.NewQuery. Add model structs using AddModel providing a pointer to an
//instance of the model type and the alias used for the model in the query.
//The query provided should not include SELECT and should start with FROM.
func ExampleQuery() {
	var db *gorm.DB

	//example structs
	type Author struct {
		AuthorID uint `gorm:"primary_key"`
		Name     string
	}

	type Exercise struct {
		ExerciseID uint `gorm:"primary_key"`
		SectionID  uint
		Name       string
	}

	type Section struct {
		SectionID  *uint `gorm:"primary_key"`
		TextbookID uint
		Title      string

		Exercises []Exercise `gorm:"foreignkey:SectionID;association_foreignkey:SectionID"`
	}

	type Textbook struct {
		TextbookID uint `gorm:"primary_key"`
		AuthorID   uint
		Name       string

		Author   Author     `gorm:"foreignkey:AuthorID;association_foreignkey:AuthorID"`
		Sections []*Section `gorm:"foreignkey:TextbookID;association_foreignkey:TextbookID"`
	}

	var textbooks []Textbook
	err := hydrate.NewQuery(db, `FROM textbooks t
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
	if err != nil {
		fmt.Printf("err = %v\n", err)
	}

	//all textbooks are fully loaded with all relationships
	fmt.Printf("%d textbooks loaded\n", len(textbooks))
	for _, t := range textbooks {
		fmt.Printf("Textbook %d has %d sections\n", t.TextbookID, len(t.Sections))
		for _, s := range t.Sections {
			fmt.Printf("Section %d has %d exercises\n", s.SectionID, len(s.Exercises))
		}
	}
}

func ExampleMultiQuery() {
	var db *gorm.DB

	//example tables
	type Author struct {
		AuthorID uint `gorm:"primary_key"`
		Name     string
	}

	type Section struct {
		SectionID  *uint `gorm:"primary_key"`
		TextbookID uint
		Title      string
	}

	type Textbook struct {
		TextbookID uint `gorm:"primary_key"`
		AuthorID   uint
		Name       string

		Author   Author     `gorm:"foreignkey:AuthorID;association_foreignkey:AuthorID"`
		Sections []*Section `gorm:"foreignkey:TextbookID;association_foreignkey:TextbookID"`
	}

	var textbooks []Textbook
	err := hydrate.MultiQuery{
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

	if err != nil {
		fmt.Printf("err = %v\n", err)
	}

	//all textbooks are fully loaded with all relationships
	fmt.Printf("%d textbooks loaded\n", len(textbooks))
	for _, t := range textbooks {
		fmt.Printf("Textbook %d has %d sections\n", t.TextbookID, len(t.Sections))
	}
}
