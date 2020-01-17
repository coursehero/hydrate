package hydrate

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

var update = flag.Bool("update", false, "update .golden files")

func GetUpdateFlag() bool {
	return *update
}

var patternNonAlphanumeric = regexp.MustCompile("[^A-Za-z0-9]")

// golden provides a way to read canned data from golden files as well as updating the files with new data.
type golden struct {
	Name   string
	update *bool
}

func (g golden) Write(data []byte) error {
	golden := g.GetFilename()
	err := ioutil.WriteFile(golden, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write golden file %s : %w", golden, err)
	}
	return nil
}

func (g golden) Read() ([]byte, error) {
	golden := g.GetFilename()
	data, err := ioutil.ReadFile(golden)
	if err != nil {
		return nil, fmt.Errorf("failed to read golden file %s : %w", golden, err)
	}
	return data, nil
}

func (g golden) GetFilename() string {
	return "testdata/" + patternNonAlphanumeric.ReplaceAllString(g.Name, "-") + ".golden"
}

// Equal will determine if the actual received content matches the value in the golden flag.
// If the update flag is provided when testing, it will update the value stored in the golden flag to match the
// content provided in actual
func (g golden) Equal(t *testing.T, actual []byte) bool {
	if (g.update == nil && GetUpdateFlag()) || (g.update != nil && *g.update) {
		buffer := bytes.Buffer{}
		_ = json.Indent(&buffer, actual, "", "\t")
		goldenData := buffer.Bytes()

		if err := g.Write(goldenData); err != nil {
			t.Errorf("Failed to update golden file, %v", err)
			return false
		}
	}

	expected, err := g.Read()
	if err != nil {
		t.Errorf("Failed to read golden file, %v", err)
		return false
	}

	if string(expected) == "" {
		return assert.Empty(t, string(actual), "expected no output")
	}

	return assert.JSONEq(t, string(expected), string(actual), "expected output did not match")
}
