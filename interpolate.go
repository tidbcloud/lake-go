package golake

import (
	"database/sql/driver"
	"github.com/pkg/errors"
	"reflect"
	"strings"
)

func buildQuery(query string, args []driver.Value, placeholders *[]int) (string, error) {
	var q string
	var err error
	if placeholders != nil {
		if len(*placeholders) != len(args) {
			return "", errors.Errorf("expect %v args, got %v", len(*placeholders), len(args))
		}
		q, err = interpolateParams2(query, args, *placeholders)
		if err != nil {
			return "", err
		}
		return q, nil
	}
	if len(args) > 0 {
		result, err := interpolateParams(query, args)
		if err != nil {
			return result, errors.Wrap(err, "buildRequest: failed to interpolate params")
		}
		return result, nil
	}
	return query, nil
}

// placeholders uses a single byte-offset scan, with the same quoted regions as
// the Lake SQL lexer. Counting and locating placeholders must never diverge.
func placeholders(query string) []int {
	var index []int
	for i := 0; i < len(query); i++ {
		switch query[i] {
		case '\'', '"', '`':
			quote := query[i]
			for i++; i < len(query); i++ {
				if query[i] == '\\' && quote != '`' {
					i++
				} else if query[i] == quote {
					if i+1 < len(query) && query[i+1] == quote {
						i++
					} else {
						break
					}
				}
			}
		case '-':
			if i+1 < len(query) && query[i+1] == '-' {
				for i += 2; i < len(query) && query[i] != '\n' && query[i] != '\f'; i++ {
				}
			}
		case '/':
			if i+1 < len(query) && query[i+1] == '*' {
				end := strings.Index(query[i+2:], "*/")
				if end < 0 {
					return index
				}
				i += end + 3
			}
		case '$':
			if i+1 < len(query) && query[i+1] == '$' {
				end := strings.Index(query[i+2:], "$$")
				if end < 0 {
					return index
				}
				i += end + 3
			}
		case '@':
			// Stage locations are unquoted literal tokens in Lake SQL.
			for i++; i < len(query); i++ {
				if query[i] == '\\' && i+1 < len(query) && strings.ContainsRune(" \t\r\n\f\v'\"\\", rune(query[i+1])) {
					i++
				} else if strings.ContainsRune(" \t\r\n\f\v,`;\"'()", rune(query[i])) {
					i--
					break
				}
			}
		case '?':
			index = append(index, i)
		}
	}
	return index
}

func interpolateParams(query string, params []driver.Value) (string, error) {
	return interpolateParams2(query, params, placeholders(query))
}

func interpolateParams2(query string, params []driver.Value, index []int) (string, error) {
	if len(params) == 0 {
		return query, nil
	}
	if params[0] != nil && reflect.TypeOf(params[0]).Kind() == reflect.Slice {
		if reflect.ValueOf(params[0]).Len() == 0 {
			return query, nil
		}
	}
	if len(index) != len(params) {
		return "", ErrPlaceholderCount
	}

	previous := -1
	for _, offset := range index {
		if offset <= previous || offset >= len(query) || query[offset] != '?' {
			return "", ErrPlaceholderCount
		}
		previous = offset
	}

	var (
		queryRaw      = []byte(query)
		paramsEncoded = make([][]byte, len(params))
		n             = len(queryRaw) - len(index) // do not count number of placeholders
	)
	for i, v := range params {
		encoded, err := textEncode.Encode(v)
		if err != nil {
			// Silently inserting the empty bytes would send a statement that no
			// longer matches what the caller bound; fail the query instead.
			return "", errors.Wrapf(err, "failed to encode parameter %d", i)
		}
		paramsEncoded[i] = encoded
		n += len(encoded)
	}
	buf := make([]byte, n)
	i := 0
	k := 0
	for j, idx := range index {
		copy(buf[k:], queryRaw[i:idx])
		k += idx - i
		copy(buf[k:], paramsEncoded[j])
		i = idx + 1
		k += len(paramsEncoded[j])
	}
	copy(buf[k:], query[i:])
	return string(buf), nil
}
