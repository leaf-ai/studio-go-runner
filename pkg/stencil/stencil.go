// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package stencil

// This files implements a simple wrapper around templating capabilities from Go
// with serveral formats for the variables that will be applied to the template and
// uses the MasterMinds sprig library for additional functions within the templates
//
// A large portion of this code is derived from an Apache 2.0 Licensed CLI utility
// that can be found at https://github.com/subchen/frep.  This file converts the
// non library packing of the original code based to be workable as a library.
//
// Portion also come from open source projects from the Authors released under Apache 2.0.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/BurntSushi/toml"
	"github.com/Masterminds/sprig/v3"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-yaml/yaml"

	"github.com/go-stack/stack" // Forked copy of https://github.com/go-stack/stack
	"github.com/jjeffery/kv"    // Forked copy of https://github.com/jjeffery/kv
)

// FuncMap augments the template functions with some standard string manipulation functions
// for document format conversions
//
func FuncMap(funcs map[string]interface{}) (f template.FuncMap) {
	// For more documentation about templating see http://masterminds.github.io/sprig/
	f = sprig.TxtFuncMap()

	// marshaling functions that be be inserted into the templated files
	f["toJson"] = toJson
	f["toYaml"] = toYaml
	f["toToml"] = toToml

	for name, fun := range funcs {
		f[name] = fun
	}
	return f
}

// toJson takes an interface, marshals it to json, and returns a string. It will
// always return a string, even on marshal error (empty string).
//
// This is designed to be called from a template.
func toJson(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		// Swallow kv.inside of a template.
		return ""
	}
	return string(data)
}

// toYaml takes an interface, marshals it to yaml, and returns a string. It will
// always return a string, even on marshal error (empty string).
//
// This is designed to be called from a template.
func toYaml(v interface{}) string {
	data, err := yaml.Marshal(v)
	if err != nil {
		// Swallow kv.inside of a template.
		return ""
	}
	return string(data)
}

// toToml takes an interface, marshals it to toml, and returns a string. It will
// always return a string, even on marshal error (empty string).
//
// This is designed to be called from a template.
func toToml(v interface{}) string {
	b := bytes.NewBuffer(nil)
	e := toml.NewEncoder(b)
	err := e.Encode(v)
	if err != nil {
		return err.Error()
	}
	return b.String()
}

// newTemplateVariables creates a template context for variable substitution etc
func newTemplateVariables(jsonVals string, loadFiles []string, overrideVals map[string]string) (vars map[string]interface{}, err kv.Error, warnings []kv.Error) {

	vars = map[string]interface{}{}

	// Env
	envs := map[string]interface{}{}
	for _, env := range os.Environ() {
		keyval := strings.SplitN(env, "=", 2)
		envs[keyval[0]] = keyval[1]
	}
	vars["Env"] = envs

	if jsonVals != "" {
		obj := map[string]interface{}{}
		if errGo := json.Unmarshal([]byte(jsonVals), &obj); errGo != nil {
			return nil, kv.Wrap(errGo, "bad json format").With("stack", stack.Trace().TrimRuntime()), warnings
		}
		for k, v := range obj {
			vars[k] = v
		}
	}

	for _, file := range loadFiles {
		if byts, errGo := ioutil.ReadFile(file); errGo != nil {
			return nil, kv.Wrap(errGo).With("file", file).With("stack", stack.Trace().TrimRuntime()), warnings
		} else {
			obj := map[string]interface{}{}

			switch filepath.Ext(file) {
			case ".json":
				if errGo := json.Unmarshal(byts, &obj); errGo != nil {
					return nil, kv.Wrap(errGo, "unrecognized json").With("file", file).With("stack", stack.Trace().TrimRuntime()), warnings
				}
			case ".yaml", ".yml":
				if errGo := yaml.Unmarshal(byts, &obj); errGo != nil {
					return nil, kv.Wrap(errGo, "unrecognized yaml").With("file", file).With("stack", stack.Trace().TrimRuntime()), warnings
				}
			case ".toml":
				if errGo := toml.Unmarshal(byts, &obj); errGo != nil {
					return nil, kv.Wrap(errGo, "unrecognized toml").With("file", file).With("stack", stack.Trace().TrimRuntime()), warnings
				}
			default:
				return nil, kv.NewError("unsupported file type (extension)").With("file", file).With("stack", stack.Trace().TrimRuntime()), warnings
			}

			for k, v := range obj {
				vars[k] = v
			}
		}
	}

	for k, v := range overrideVals {

		// remove quotes for key="value"
		if strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"") {
			v = v[1 : len(v)-1]
		} else if strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'") {
			v = v[1 : len(v)-1]
		}
		vars[k] = v
	}

	return vars, nil, warnings
}

func templateExecute(t *template.Template, src io.Reader, dest io.Writer, ctx interface{}) (err kv.Error) {

	readBytes, errGo := ioutil.ReadAll(src)
	if errGo != nil {
		return kv.Wrap(errGo, "pasing failed for template file(s)").With("stack", stack.Trace().TrimRuntime())
	}

	tmpl, errGo := t.Parse(string(readBytes))
	if errGo != nil {
		return kv.Wrap(errGo, "pasing failed for template file(s)").With("stack", stack.Trace().TrimRuntime())
	}

	if errGo = tmpl.Execute(dest, ctx); errGo != nil {
		return kv.Wrap(errGo, "output file could not be created").With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

// TemplateIOFiles is used to encapsulate some streaming interfaces for input and output documents
type TemplateIOFiles struct {
	In  io.Reader
	Out io.Writer
}

// TemplateOptions is used to pass into the Template function both streams and key values
// for the template engine
type TemplateOptions struct {
	IOFiles        []TemplateIOFiles
	Delimiters     []string
	ValueFiles     []string
	OverrideValues map[string]string
}

// Template takes the TemplateOptions and processes the template execution, it also
// it used to catch and report errors the user raises within the template from
// validation checking etc
//
func Template(opts TemplateOptions) (err kv.Error, warnings []kv.Error) {

	defer func() {
		if err := recover(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()

	tmplErrs := []kv.Error{}
	funcs := template.FuncMap{
		"RaiseError": func(msg string) string {
			tmplErrs = append(tmplErrs, kv.NewError(msg).With("stack", stack.Trace().TrimRuntime()))
			return ""
		},
	}

	t := template.New("noname").Funcs(FuncMap(funcs))

	if len(opts.Delimiters) != 0 {
		if len(opts.Delimiters) != 2 {
			return kv.NewError("unexpected number of delimiters, two are expected [\"left\",\"right\"").With("stack", stack.Trace().TrimRuntime()), warnings
		}
		t = t.Delims(opts.Delimiters[0], opts.Delimiters[1])
	}

	vars, err, warnings := newTemplateVariables("", opts.ValueFiles, opts.OverrideValues)
	if err != nil {
		return err, warnings
	}

	fmt.Println(spew.Sdump(vars))
	for _, files := range opts.IOFiles {
		err = templateExecute(t, files.In, files.Out, vars)
		if err != nil {
			return err, warnings
		}
	}
	if len(tmplErrs) != 0 {
		return tmplErrs[0], tmplErrs
	}

	return nil, warnings
}
