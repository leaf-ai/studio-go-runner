package main

import (
	"testing"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
	"github.com/leaf-ai/studio-go-runner/internal/runner"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/go-test/deep"
)

// TestAJSONMergePatch is used to exercise the IETF Merge patch document for
// https://tools.ietf.org/html/rfc7386
//
func TestAJSONMergePatch(t *testing.T) {

	x1 := runner.ExprJSON{
		Experiment: map[string]interface{}{
			"D": "d",
			"C": "c",
			"B": "b",
			"A": "a",
		},
	}
	x2 := runner.ExprJSON{
		Experiment: map[string]interface{}{
			"A": 1,
			"E": 2,
		},
	}

	expected1 := `{ "experiment": { "A": "a", "B": "b", "C": "c", "D": "d", "E": 2 } }`
	doc1, err := runner.ExtractMergeDoc(x1, x2)
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(expected1, doc1); diff != nil {
		t.Fatal(errors.New("JSON Merge Patch RFC 7386 Test failed").With("diff", diff, "stack", stack.Trace().TrimRuntime()))
	}

	expected2 := `{ "experiment": { "A": 1, "B": "b", "C": "c", "D": "d", "E": 2 } }`
	doc2, err := runner.ExtractMergeDoc(x2, x1)
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(expected2, doc2); diff != nil {
		t.Fatal(errors.New("JSON Merge Patch RFC 7386 Test failed").With("diff", diff, "stack", stack.Trace().TrimRuntime()))
	}
}

// TestAJSONPatch exercises a simple test case for the https://tools.ietf.org/html/rfc6902
//
func TestAJSONPatch(t *testing.T) {

	original := []byte(`{"experiment": {"name": "John", "age": 24, "height": 3.21}}`)
	patchJSON := []byte(`[
		        {"op": "replace", "path": "/experiment/name", "value": "Jane"},
				{"op": "remove", "path": "/experiment/height"}
					]`)

	patch, errGo := jsonpatch.DecodePatch(patchJSON)
	if errGo != nil {
		t.Fatal(errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	modified, errGo := patch.Apply(original)
	if errGo != nil {
		t.Fatal(errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	expected := `{"experiment":{"age":24,"name":"Jane"}}`
	if diff := deep.Equal(expected, string(modified)); diff != nil {
		t.Fatal(errors.New("JSON Patch RFC 6902 Test failed").With("diff", diff, "stack", stack.Trace().TrimRuntime()))
	}
}

// TestAJSONEditor Will put together some merge style patches and some editor style patches and run these
// through the runners internal editor
func TestAJSONzEditor(t *testing.T) {

	type testCase struct {
		directive string
		expected  string
	}
	// A table driven test is used with progressive edits and merges
	testCases := []testCase{
		testCase{
			`{"experiment": {"name": "testExpr", "max_run_length": 24, "run_length": 3.21}}`,
			`{"experiment": {"name": "testExpr", "max_run_length": 24, "run_length": 3.21}}`,
		},
		testCase{
			`[{"op": "replace", "path": "/experiment/name", "value": "testExpr1"}]`,
			`{"experiment": {"name": "testExpr1", "max_run_length": 24, "run_length": 3.21}}`,
		},
		testCase{
			`[{"op": "remove", "path": "/experiment/max_run_length"}]`,
			`{"experiment": {"name": "testExpr1", "run_length": 3.21}}`,
		},
		testCase{
			`[{"op": "add", "path": "/experiment/addition_1", "value": "additional data 1"}]`,
			`{"experiment": {"name": "testExpr1", "run_length": 3.21, "addition_1":"additional data 1"}}`,
		},
		testCase{
			`{"experiment": {"addition_2": "additional data 2"}}`,
			`{"experiment": {"name": "testExpr1", "run_length": 3.21, "addition_1":"additional data 1", "addition_2": "additional data 2"}}`,
		},
	}

	doc := "{}"
	// run one test at a time
	for _, testCase := range testCases {
		newDoc, err := runner.ExprJsonEditor(doc, []string{testCase.directive})
		if err != nil {
			t.Fatal(err)
		}
		if !jsonpatch.Equal([]byte(newDoc), []byte(testCase.expected)) {
			t.Fatal(errors.New("JSON Editor Test failed").With("expected", testCase.expected, "actual", newDoc, "stack", stack.Trace().TrimRuntime()))
		}
		doc = newDoc
	}

	// re-run tests in incremental batches
	for limit := 1; limit != len(testCases)-1; limit++ {
		doc = "{}"
		directives := []string{}
		for _, testCase := range testCases[0:limit] {
			directives = append(directives, testCase.directive)
		}
		doc, err := runner.ExprJsonEditor(doc, directives)
		if err != nil {
			t.Fatal(err)
		}
		if !jsonpatch.Equal([]byte(doc), []byte(testCases[limit-1].expected)) {
			t.Fatal(errors.New("JSON Editor Test failed").With("expected", testCases[limit-1].expected, "actual", doc, "stack", stack.Trace().TrimRuntime()))
		}
	}
}
