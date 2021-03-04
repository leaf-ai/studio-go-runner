package defense

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// This file contains test cases for the file system checking code

type escapeTest struct {
	target    string
	candidate string
	escapes   bool
}

var (
	testCases = []escapeTest{
		escapeTest{
			target:    "/tmp",
			candidate: "x",
			escapes:   false,
		},
		escapeTest{
			target:    "/tmp",
			candidate: "../x",
			escapes:   true,
		},
		escapeTest{
			target:    "/tmp",
			candidate: "../tmp/x",
			escapes:   false,
		},
	}
)

func TestWillEscape(t *testing.T) {
	for _, testCase := range testCases {
		if escapes, err := WillEscape(testCase.candidate, testCase.target); escapes != testCase.escapes {
			if err != nil {
				t.Fatal(err.Error())
			}
			if testCase.escapes {
				t.Fatal("escape not detected", spew.Sdump(testCase))
			} else {
				t.Fatal("non-existant escape detected", spew.Sdump(testCase))
			}
		}
	}
}
