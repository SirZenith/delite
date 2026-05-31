package utils

import (
	"fmt"
	"reflect"
	"regexp"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

// GoValueToLuaValue converts a Go value to lua.LValue.
func GoValueToLuaValue(value any) (lua.LValue, error) {
	var err error

	lValue, ok := value.(lua.LValue)
	if ok {
		return lValue, err
	}

	switch v := value.(type) {
	case int, int16, int32, int64, float32, float64:
		lValue = lua.LNumber(reflect.ValueOf(v).Convert(reflect.TypeOf(float64(0))).Float())
	case string:
		lValue = lua.LString(v)
	case bool:
		if v {
			lValue = lua.LTrue
		} else {
			lValue = lua.LFalse
		}
	default:
		err = fmt.Errorf("unsupported element value type: %T", value)
		lValue = lua.LNil
	}

	return lValue, err
}

var (
	patternMultipleWhitespace     *regexp.Regexp
	oncePatternMultipleWhitespace sync.Once
)

// GetMultipleWhitespacePattern returns a compiled regular expression that matches
// one or more whitespace.
func GetMultipleWhitespacePattern() *regexp.Regexp {
	oncePatternMultipleWhitespace.Do(func() {
		patternMultipleWhitespace = regexp.MustCompile(`\s+`)
	})

	return patternMultipleWhitespace
}
