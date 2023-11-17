// Copyright 2018-2022 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"github.com/andreidenissov-cog/go-service/pkg/log"
	"regexp"
)

type OutputFilter interface {
	Filter(input []byte) []byte
}

var (
	logFilterExpr1    = "(git\\+https:|git:)//(.*?)@github.com"
	logFilterExpr2    = "(\\w*?)(CREDENTIAL|PASSWORD|PASSWD|TOKEN|SECRET|KEY)(\\w*?)=(.*)(?m)$"
	logFilterReplace1 = []byte("${1}//****@github.com")
	logFilterReplace2 = []byte("${1}${2}${3}=****")
)

type LogFilterer struct {
	expr1 *regexp.Regexp
	expr2 *regexp.Regexp
}

func (lf *LogFilterer) Filter(input []byte) []byte {
	if lf.expr1 != nil {
		input = lf.expr1.ReplaceAll(input, logFilterReplace1)
	}
	if lf.expr2 != nil {
		input = lf.expr2.ReplaceAll(input, logFilterReplace2)
	}
	return input
}

func compileExpr(expr string, logger *log.Logger) (result *regexp.Regexp) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("FAILED to compile regexpr!", "regexpr:", expr, "reason:", r)
			result = nil
		}
	}()

	result = regexp.MustCompile(expr)
	return result
}

func GetLogFilterer(logger *log.Logger) OutputFilter {
	filter := &LogFilterer{}
	filter.expr1 = compileExpr(logFilterExpr1, logger)
	filter.expr2 = compileExpr(logFilterExpr2, logger)
	return filter
}
