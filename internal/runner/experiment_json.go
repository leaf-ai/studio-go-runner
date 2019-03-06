package runner

// This file contains functions capable of processing experiment json structures

import (
	"encoding/json"
	"strings"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	jsonpatch "github.com/evanphx/json-patch"
)

// mergeExperiment merges the two JSON-marshalable values x1 and x2,
// preferring x1 over x2 except where x1 and x2 are
// JSON objects, in which case the keys from both objects
// are included and their values merged recursively.
//
// It returns an error if x1 or x2 cannot be JSON-marshaled.
//
func MergeExperiment(x1, x2 interface{}) (interface{}, kv.Error) {
	data1, errGo := json.Marshal(x1)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	data2, errGo := json.Marshal(x2)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	var j1 interface{}
	errGo = json.Unmarshal(data1, &j1)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	var j2 interface{}
	errGo = json.Unmarshal(data2, &j2)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return mergeMaps(j1, j2), nil
}

func mergeMaps(x1, x2 interface{}) interface{} {
	switch x1 := x1.(type) {
	case map[string]interface{}:
		x2, ok := x2.(map[string]interface{})
		if !ok {
			return x1
		}
		for k, v2 := range x2 {
			if v1, ok := x1[k]; ok {
				x1[k] = mergeMaps(v1, v2)
			} else {
				x1[k] = v2
			}
		}
	case nil:
		// merge(nil, map[string]interface{...}) -> map[string]interface{...}
		x2, ok := x2.(map[string]interface{})
		if ok {
			return x2
		}
	}
	return x1
}

func ExtractMergeDoc(x1, x2 interface{}) (results string, err kv.Error) {
	x3, err := MergeExperiment(x1, x2)
	if err != nil {
		return "", err
	}

	data, errGo := json.MarshalIndent(x3, "", "\t")
	if errGo != nil {
		return "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	lines := []string{}
	for _, aLine := range strings.Split(string(data), "\n") {
		lines = append(lines, strings.TrimSpace(aLine))
	}

	return strings.Join(lines, " "), nil
}

// JSONEditor will accept a source JSON document and an array of change edits
// for the source document and will process them as either RFC7386, or RFC6902 edits
// if they validate as either.
//
func JSONEditor(oldDoc string, directives []string) (result string, err kv.Error) {

	doc := []byte(oldDoc)

	if len(doc) == 0 {
		doc = []byte(`{}`)
	}

	for _, directive := range directives {
		patch, errGo := jsonpatch.DecodePatch([]byte(directive))
		if errGo == nil {
			if doc, errGo = patch.Apply(doc); errGo != nil {
				return "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		} else {
			var edit interface{}
			if errGo = json.Unmarshal([]byte(directive), &edit); errGo != nil {
				return "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
			var sourceDoc interface{}
			if errGo = json.Unmarshal([]byte(doc), &sourceDoc); errGo != nil {
				return "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
			extracted, err := ExtractMergeDoc(&edit, &sourceDoc)
			if err != nil {
				return "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
			doc = []byte(extracted)
		}
	}

	return string(doc), nil
}
