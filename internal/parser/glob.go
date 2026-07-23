package parser

import (
	"fmt"
	"regexp"
	"strings"
)

func MatchGlob(pattern, target string) (bool, error) {
	pattern = strings.TrimSpace(strings.ReplaceAll(pattern, "\\", "/"))
	target = strings.TrimPrefix(strings.ReplaceAll(target, "\\", "/"), "./")
	if pattern == "" {
		return false, nil
	}
	expression, err := globRegexp(pattern)
	if err != nil {
		return false, err
	}
	return regexp.MatchString(expression, target)
}

func globRegexp(pattern string) (string, error) {
	var out strings.Builder
	out.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					out.WriteString("(?:.*/)?")
				} else {
					out.WriteString(".*")
				}
			} else {
				out.WriteString("[^/]*")
			}
		case '?':
			out.WriteString("[^/]")
		case '{':
			end := strings.IndexByte(pattern[i+1:], '}')
			if end < 0 {
				return "", fmt.Errorf("unclosed brace in glob")
			}
			end += i + 1
			items := strings.Split(pattern[i+1:end], ",")
			if len(items) < 2 {
				return "", fmt.Errorf("brace glob requires alternatives")
			}
			out.WriteString("(?:")
			for index, item := range items {
				if index > 0 {
					out.WriteString("|")
				}
				out.WriteString(regexp.QuoteMeta(item))
			}
			out.WriteString(")")
			i = end
		default:
			out.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	out.WriteString("$")
	return out.String(), nil
}
