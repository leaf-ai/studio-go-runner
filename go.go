package runner

// This file contains function(s) useful for running the test suite.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

// GoGetConst will retrieve data structures from source code within the
// code directories that can contain useful information to utilities
// visiting the code for testing purposes.  It is used mainly to
// retrieve command line parameters used during testing that packages contain
// so that when tests are run by external application neutral software the
// code under test can parameterize itself.
//
func GoGetConst(dir string, constName string) (v [][]string, err errors.Error) {

	fset := token.NewFileSet()
	parserMode := parser.ParseComments

	errGo := filepath.Walk(dir, func(file string, fi os.FileInfo, err error) error {
		if v != nil {
			return nil
		}
		if fi.IsDir() {
			// Dont recurse into directories with a main, needs AST treatment to discover main(s)
			if fi.Name() == "cmd" || fi.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(fi.Name(), ".go") {
			return nil
		}
		fileAst, errGo := parser.ParseFile(fset, file, nil, parserMode)
		if errGo != nil {
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		for _, d := range fileAst.Decls {
			switch decl := d.(type) {
			case *ast.FuncDecl:
				continue
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					switch spec := spec.(type) {
					case *ast.ImportSpec:
						continue
					case *ast.TypeSpec:
						continue
					case *ast.ValueSpec:
						for _, id := range spec.Names {
							if id.Name == constName {
								opts := []string{}
								for _, value := range spec.Values {
									for _, lts := range value.(*ast.CompositeLit).Elts {
										if strs, ok := lts.(*ast.CompositeLit); ok {
											for _, lt := range strs.Elts {
												if entry, ok := lt.(*ast.BasicLit); ok {
													opts = append(opts, entry.Value)
												}
											}
										}
									}
								}
								v = append(v, opts)
							}
						}
					default:
						fmt.Printf("Unknown token type: %s\n", decl.Tok)
					}
				}
			default:
				fmt.Print("Unknown declaration @\n", decl.Pos())
			}
		}
		return nil
	})
	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return v, nil
}
