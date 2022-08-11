// Copyright 2018-2022 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import "regexp"

type OutputFilter interface {
	Filter(input []byte) []byte
}

var (
	logFilterExpr    = "git+https://(.*)@github.com"
	logFilterReplace = []byte("git+https://****************@github.com")
)

type LogFilterer struct {
	expr *regexp.Regexp
}

func (lf *LogFilterer) Filter(input []byte) []byte {
	if lf.expr != nil {
		return lf.expr.ReplaceAll(input, logFilterReplace)
	}
	return input
}

func GetLogFilterer() OutputFilter {
	filter := &LogFilterer{}

	defer func() {
		if r := recover(); r != any(nil) {
			filter.expr = nil
		}
	}()

	filter.expr = regexp.MustCompile(logFilterExpr)
	return filter
}
