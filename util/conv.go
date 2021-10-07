package util

import "time"

func ToDuration(val interface{}) time.Duration {
	switch val := val.(type) {
	case nil:
		return 0
	case int:
		return time.Duration(val)
	case int64:
		return time.Duration(val)
	case string:
		if len(val) == 0 {
			return 0
		}
		if ret, err := time.ParseDuration(val); err == nil {
			return ret
		}
	case time.Duration:
		return val
	}
	//panic(fmt.Sprintf("invalid value to duration: %v", val))
	return 0
}
